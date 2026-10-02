package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"reflexo/internal/device"
	"reflexo/internal/fetch"
	"reflexo/internal/guide"
	"reflexo/internal/i18n"
	"reflexo/internal/logfile"
)

// fakeClock lets the whole demonstration be inspected without waiting for it.
//
// The timeline is half a minute long. A test that had to sleep that long would
// be a test nobody runs, and the states it covers are the ones a visitor sees
// in the first five seconds.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

// TestHelperFakeADB is not a real test.
//
// It is the child process that stands in for adb. Re-executing the test binary
// with REFLEXO_FAKE_ADB set gives the device client something that speaks the
// adb protocol, which is the only way to prove that a genuinely connected
// phone takes precedence over the demonstration.
func TestHelperFakeADB(t *testing.T) {
	if os.Getenv("REFLEXO_FAKE_ADB") != "1" {
		t.Skip("processo auxiliar; só roda quando reexecutado como adb")
	}

	args := strings.Join(os.Args[1:], " ")

	switch {
	case strings.Contains(args, "start-server"):
		os.Stdout.WriteString("* daemon started\n")

	case strings.Contains(args, "devices"):
		if os.Getenv("REFLEXO_FAKE_ADB_STATE") == "" {
			// The visitor's situation: nothing plugged in at all.
			os.Stdout.WriteString("List of devices attached\n")
			break
		}
		os.Stdout.WriteString("List of devices attached\n")
		os.Stdout.WriteString("FAKE123\t" + os.Getenv("REFLEXO_FAKE_ADB_STATE") + "\n")

	case strings.Contains(args, "getprop"):
		os.Stdout.WriteString("[ro.product.manufacturer]: [Google]\n")
		os.Stdout.WriteString("[ro.product.model]: [Pixel 7]\n")
		os.Stdout.WriteString("[ro.build.version.release]: [14]\n")
		os.Stdout.WriteString("[ro.build.version.sdk]: [34]\n")

	default:
		os.Stdout.WriteString("\n")
	}
}

// fakeADB points an App at a device client backed by the child process above.
func fakeADB(t *testing.T, state string) *device.Client {
	t.Helper()

	stateEnv := ""
	if state != "" {
		stateEnv = "REFLEXO_FAKE_ADB_STATE=" + state
	}

	// #nosec G204 -- the test binary itself, with a test-run filter.
	script := "@echo off\r\n" +
		"set REFLEXO_FAKE_ADB=1\r\n" +
		"set " + stateEnv + "\r\n" +
		"\"" + os.Args[0] + "\" -test.run=^TestHelperFakeADB$ %*\r\n"

	path := filepath.Join(t.TempDir(), "adb.cmd")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("não foi possível criar o adb falso: %v", err)
	}
	return device.NewClient(path)
}

// newDemoApp returns an App playing the demonstration, on a clock the test owns.
func newDemoApp(t *testing.T) (*App, *fakeClock) {
	t.Helper()

	clock := &fakeClock{now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}

	a := New(t.TempDir())
	a.StartDemo()

	a.mu.Lock()
	a.demo.now = clock.Now
	a.demo.start = clock.Now()
	a.mu.Unlock()

	return a, clock
}

func demoLogDir(t *testing.T) (*App, string) {
	t.Helper()

	dir := t.TempDir()
	lg, err := logfile.Open(dir, "reflexo.log")
	if err != nil {
		t.Fatalf("logfile.Open(): %v", err)
	}
	t.Cleanup(func() { lg.Close() })

	a := New(t.TempDir())
	a.SetLogger(lg)
	return a, dir
}

// The demonstration must describe its fake phone with exactly the code that
// describes a real one.
//
// This is the test that keeps the demonstration from becoming a lie. A demo
// built from its own copy of the text would agree with the product until the
// first time either was edited, and would then advertise something Reflexo
// does not do — which is worse than having no demonstration at all.
func TestDemoDescribesFakePhoneThroughTheRealGuide(t *testing.T) {
	a, clock := newDemoApp(t)

	// Move to the permission step, where the guide has real work to do.
	clock.advance(demoTimeline[0].hold)

	devices, _ := a.demo.moment()
	if len(devices) != 1 || devices[0].State != device.StateUnauthorized {
		t.Fatalf("a demonstração deveria estar em 'unauthorized', veio %v", devices)
	}

	// What the App renders...
	a.Refresh(context.Background())
	shown := a.Snapshot()

	// ...against what the product would show for a phone in that same state.
	want := guide.Of(situationFor(devices), i18n.Detect())

	if shown.Headline != want.Headline {
		t.Errorf("a demonstração mostrou %q, mas o guia real produziria %q",
			shown.Headline, want.Headline)
	}
	if shown.Detail != want.Detail {
		t.Errorf("a demonstração mostrou um detalhe diferente do guia real:\n  demo: %q\n  real: %q",
			shown.Detail, want.Detail)
	}
	if shown.Severity != int(want.Severity) {
		t.Errorf("severidade %d na demonstração, %d no guia real",
			shown.Severity, int(want.Severity))
	}
	if strings.Join(shown.Steps, "|") != strings.Join(want.Steps, "|") {
		t.Errorf("passos diferentes:\n  demo: %v\n  real: %v", shown.Steps, want.Steps)
	}
}

// The demonstration has to tell the truth about itself. A visitor who cannot
// tell a scripted phone from their own has been misled by something Reflexo
// chose to do.
func TestDemoAnnouncesItself(t *testing.T) {
	a, _ := newDemoApp(t)
	a.Refresh(context.Background())

	s := a.Snapshot()
	if !s.Demo {
		t.Error("State.Demo = false durante a demonstração; a página não avisaria que é uma simulação")
	}
	if s.CanDemo {
		t.Error("State.CanDemo = true durante a demonstração; o botão ofereceria outra demonstração")
	}
}

// The button is the whole first impression: it must appear exactly when there
// is nothing else to show, and stay out of the way otherwise.
func TestDemoOfferedOnlyWhenThereIsNothingToShow(t *testing.T) {
	// The visitor's situation: nothing plugged in.
	a := New(t.TempDir())
	a.Refresh(context.Background())
	if !a.Snapshot().CanDemo {
		t.Error("sem aparelho, o Reflexo deveria oferecer a demonstração")
	}

	// A real phone turns up: the button must go away, or it competes with the
	// thing the user actually came for.
	b := New(t.TempDir())
	b.adb = fakeADB(t, "device")
	b.Refresh(context.Background())

	s := b.Snapshot()
	if s.CanDemo {
		t.Error("com aparelho conectado, o Reflexo não deveria oferecer a demonstração")
	}
	if s.Demo {
		t.Error("State.Demo = true sem haver demonstração")
	}
	if !s.CanStart {
		t.Error("com aparelho autorizado, deveria ser possível espelhar")
	}
}

// A real phone outranks the script. Someone who started the demonstration and
// then plugged in their phone wants their phone, and a loop that ignores it is
// the most damaging impression the product could give.
func TestRealDeviceEndsTheDemo(t *testing.T) {
	a, _ := newDemoApp(t)

	// Nothing plugged in yet: the demonstration holds.
	a.adb = fakeADB(t, "")
	a.Refresh(context.Background())
	if !a.Demoing() {
		t.Fatal("sem aparelho, a demonstração deveria continuar")
	}
	if !a.Snapshot().Demo {
		t.Error("a página deveria avisar que estáShowing uma simulação")
	}

	// Now the phone shows up.
	a.adb = fakeADB(t, "device")
	a.Refresh(context.Background())

	if a.Demoing() {
		t.Error("um aparelho real deveria ter encerrado a demonstração")
	}
	s := a.Snapshot()
	if s.Demo {
		t.Error("State.Demo continuou true depois de um aparelho real aparecer")
	}
	if !s.CanStart {
		t.Error("depois de um aparelho real, o espelhamento deveria estar disponível")
	}
}

// The demonstration loops. A script that stops on a screen is a screenshot; one
// that keeps moving is a person watching, and they have no idea what to do next
// if it freezes.
func TestDemoLoopsForever(t *testing.T) {
	a, clock := newDemoApp(t)

	seen := map[string]bool{}
	for i := 0; i < int(demoLoop*3/time.Second); i++ {
		devices, mirroring := a.demo.moment()
		seen[demoStateName(devices, mirroring)] = true
		clock.advance(time.Second)
	}

	for _, want := range []string{"none", "unauthorized", "ready", "mirroring"} {
		if !seen[want] {
			t.Errorf("a demonstração nunca passou por %q", want)
		}
	}
}

func demoStateName(devices []device.Device, mirroring bool) string {
	switch {
	case mirroring:
		return "mirroring"
	case len(devices) == 0:
		return "none"
	case devices[0].State == device.StateUnauthorized:
		return "unauthorized"
	default:
		return "ready"
	}
}

// The demonstration walks through the story a first-time user actually has,
// including the permission prompt, which is where beginners give up on scrcpy
// and where Reflexo's value is clearest.
func TestDemoTellsTheWholeStory(t *testing.T) {
	a, clock := newDemoApp(t)

	var order []string
	for _, step := range demoTimeline {
		devices, mirroring := a.demo.moment()
		order = append(order, demoStateName(devices, mirroring))
		clock.advance(step.hold)
	}

	want := []string{"none", "unauthorized", "ready", "mirroring"}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Errorf("a sequência foi %v, esperado %v", order, want)
	}
}

// Pressing the button during the demonstration must move the screen. A click
// that silently does nothing reads as a broken button — the exact opposite of
// what the demonstration is for.
func TestStartDuringDemoJumpsToMirroring(t *testing.T) {
	a, _ := newDemoApp(t)
	a.Refresh(context.Background())

	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start(): %v", err)
	}

	s := a.Snapshot()
	if !s.Running {
		t.Error("após o clique, a demonstração não mostrou o espelhamento")
	}
	if !s.CanStart {
		t.Error("o espelhamento da demonstração deveria habilitar o botão")
	}
}

// Nothing may be launched during a demonstration: there is no phone, and
// starting scrcpy anyway would either flash an error window or, worse, attach
// to whatever device happens to be plugged in.
func TestStartDuringDemoLaunchesNothing(t *testing.T) {
	a, _ := newDemoApp(t)

	launched := false
	a.launch = func(*fetch.Layout, []string) (*exec.Cmd, error) {
		launched = true
		return nil, nil
	}

	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start(): %v", err)
	}

	if launched {
		t.Error("a demonstração tentou montar um comando de scrcpy")
	}

	a.mu.RLock()
	proc := a.scrcpy
	a.mu.RUnlock()
	if proc != nil {
		t.Error("a demonstração guardou um processo")
	}
}

// The fake phone must never reach the support log. Writing DEMO0001 next to
// real diagnostics would put a fabrication in the one file whose entire value
// is that it can be believed.
func TestDemoNeverPollutesTheLogWithAFakePhone(t *testing.T) {
	dir := t.TempDir()
	lg, err := logfile.Open(dir, "reflexo.log")
	if err != nil {
		t.Fatalf("logfile.Open(): %v", err)
	}
	t.Cleanup(func() { lg.Close() })

	clock := &fakeClock{now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	a := New(t.TempDir())
	a.SetLogger(lg)
	a.StartDemo()
	a.mu.Lock()
	a.demo.now = clock.Now
	a.demo.start = clock.Now()
	a.mu.Unlock()

	// Several full cycles, at the poll rate.
	for i := 0; i < 120; i++ {
		a.Refresh(context.Background())
		clock.advance(time.Second)
	}

	body := readLog(t, dir)
	if strings.Contains(body, "DEMO0001") {
		t.Errorf("a demonstração escreveu o aparelho fictício no log de suporte:\n%s", body)
	}
	// It should still have recorded that the demonstration is running, or the
	// log would show an unexplained blank.
	if !strings.Contains(body, "demonstração") {
		t.Errorf("o log não registrou a demonstração:\n%s", body)
	}
}

// readLog returns the contents of a log file, for asserting on it.
func readLog(t *testing.T, dir string) string {
	t.Helper()

	b, err := os.ReadFile(filepath.Join(dir, "reflexo.log"))
	if err != nil {
		t.Fatalf("não foi possível ler o log: %v", err)
	}
	return string(b)
}

// The demonstration is offered before scrcpy is installed, because the person
// who needs it is by definition someone who has not installed anything yet.
func TestWatchDoesNotWaitForInstallWhenDemoing(t *testing.T) {
	a, _ := newDemoApp(t)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		a.Watch(ctx)
		close(done)
	}()

	// One poll interval proves Watch is running rather than parked on the
	// install channel: without the state update, the wait below would hang.
	select {
	case <-done:
		t.Fatal("Watch retornou; deveria continuar enquanto o contexto estiver vivo")
	case <-time.After(pollInterval + 500*time.Millisecond):
	}

	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Watch não respondeu ao cancelamento")
	}

	if !a.Snapshot().Demo {
		t.Error("Watch não atualizou o estado durante a demonstração")
	}
}

// Without a demonstration, Watch still waits for the install. A regression here
// would mean polling a nil adb and freezing the page on the first run.
func TestWatchStillWaitsForInstallWhenNotDemoing(t *testing.T) {
	a := New(t.TempDir())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		a.Watch(ctx)
		close(done)
	}()

	// markReady is never called, so Watch must stay parked.
	select {
	case <-done:
		t.Fatal("Watch saiu sem esperar a instalação")
	case <-time.After(300 * time.Millisecond):
	}

	// Now release it.
	a.markReady()
	select {
	case <-done:
		t.Fatal("Watch deveria continuar depois da instalação")
	case <-time.After(200 * time.Millisecond):
	}

	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Watch não respondeu ao cancelamento")
	}
}

// Stop must leave the screen somewhere sensible rather than frozen on the
// mirroring step, because in the demonstration there is no process to kill.
func TestStopDuringDemoRestartsTheScript(t *testing.T) {
	a, _ := newDemoApp(t)

	a.Start(context.Background())
	if _, mirroring := a.demo.moment(); !mirroring {
		t.Fatal("não chegou ao espelhamento")
	}

	a.Stop()

	if _, mirroring := a.demo.moment(); mirroring {
		t.Error("Stop deixou a demonstração parada no passo de espelhamento")
	}
}

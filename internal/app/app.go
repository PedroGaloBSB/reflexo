// Package app wires the pieces together: it installs scrcpy once, keeps a live
// view of the connected device, and exposes both to a local web interface.
package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"reflexo/internal/catalog"
	"reflexo/internal/device"
	"reflexo/internal/fetch"
	"reflexo/internal/guide"
	"reflexo/internal/i18n"
	"reflexo/internal/logfile"
)

// severityName gives the numeric severity a name for the log file. The browser
// gets the catalog key; the log gets this, because a support bundle is read by
// a human, not by a renderer.
func severityName(s guide.Severity) string {
	if k := s.Key(); k != "" {
		return strings.TrimPrefix(k, "severity.")
	}
	return "desconhecido"
}

// pollInterval is how often the device list is refreshed.
//
// Two seconds is a deliberate compromise: fast enough that plugging a cable
// feels instant, slow enough that adb is not spawned constantly on a machine
// that may be running on battery.
const pollInterval = 2 * time.Second

// State is the complete picture rendered by the UI.
type State struct {
	// Setup describes the one-time installation of scrcpy.
	Setup    string  `json:"setup"` // installing | ready | error
	SetupMsg string  `json:"setupMsg"`
	SetupPct float64 `json:"setupPct"`

	// Instruction is the contextual guidance from the guide package.
	Severity int      `json:"severity"`
	Headline string   `json:"headline"`
	Detail   string   `json:"detail"`
	Steps    []string `json:"steps"`
	CanStart bool     `json:"canStart"`

	// SeverityKey is the catalog key for Severity's badge label.
	//
	// The number is sent alongside it only so the page can pick a colour
	// (dataset.sev); it must never be used to build a label, because the
	// browser cannot know that 0 means "idle" and 2 means "warn".
	SeverityKey string `json:"severityKey"`

	// Identity of the running environment.
	Platform  string `json:"platform"`
	ScrcpyVer string `json:"scrcpyVersion"`

	// Text holds the strings the browser renders itself, keyed by i18n key.
	//
	// The page cannot read a Go package, so chrome that is not part of the
	// instruction — the button label, the severity badge, the footer — travels
	// alongside the state. Everything in it is translated server-side; app.js
	// contains no user-facing words at all.
	Text map[string]string `json:"text"`

	// Running reports whether a scrcpy process is currently mirroring.
	Running bool `json:"running"`

	// ADBRecovering reports that Reflexo is restarting the adb server after it
	// stopped answering. The page needs it so the screen can say what is being
	// done instead of showing the same dead-end message it always showed.
	ADBRecovering bool `json:"adbRecovering"`

	// Demo reports that the guidance on screen comes from the demonstration
	// rather than from a real phone. The page has to say so: a visitor who
	// thinks the product is talking about their own phone has been misled by
	// something Reflexo chose to do.
	Demo bool `json:"demo"`

	// CanDemo reports whether offering the demonstration would help — that is,
	// there is no phone to talk about and the demonstration is not already
	// playing.
	CanDemo bool `json:"canDemo"`

	// Fatal is set when Reflexo cannot continue at all.
	Fatal string `json:"fatal"`
}

// App holds the running state.
type App struct {
	layout *fetch.Layout
	adb    *device.Client

	// lang is fixed for the lifetime of the App: switching language under the
	// user mid-read is worse than picking the wrong one (ADR-0006).
	lang i18n.Lang

	// ExtraArgs are appended to every scrcpy invocation, so a user can override
	// defaults from the command line without changing code.
	ExtraArgs []string

	// ready is closed once adb is usable. Watch blocks on it rather than
	// polling a nil field: on first run the install takes seconds, and a
	// Watch that started immediately would see a nil adb, return, and leave
	// the UI frozen forever.
	ready     chan struct{}
	readyOnce sync.Once

	// startMu serialises Start end to end. It is separate from mu because the
	// launch holds it across an exec, and holding the state lock that long
	// would stall the poller and freeze the UI for the duration.
	startMu sync.Mutex

	mu    sync.RWMutex
	state State

	// log receives run events. Nil is fine: every logfile method tolerates a
	// nil receiver, so Reflexo still works if logging could not start.
	log *logfile.Logger

	// lastSig is the previous situation signature, used to log only changes.
	// Polling runs every 2 seconds, so logging every result would produce 1800
	// identical lines an hour and bury the one line that matters.
	lastSig string

	// adbFailures counts consecutive polls in which adb could not be reached.
	// adbRecovering is true while a restart is in flight; lastRecovery and
	// recoveryTried drive the backoff, and mirrorLostAt remembers when the
	// mirror died so a suspicious empty list can be recognised.
	//
	// These exist because adb used to fail silently and permanently. The UI
	// said "ADB unavailable" and stayed there while a phone that worked
	// perfectly sat on the desk. See internal/device/health.go.
	startedAt     time.Time
	adbFailures   int
	adbRecovering bool
	adbKind       device.FailureKind
	lastRecovery  time.Time
	recoveryTried int
	mirrorLostAt  time.Time

	// lastDeviceSig does the same for the raw adb device list, so the log
	// shows when a cable was plugged or unplugged even when the resulting
	// instruction happens to be the same.
	lastDeviceSig string

	scrcpy *exec.Cmd

	// launch builds the mirroring command. Nil means launchScrcpy; tests
	// replace it with a process that stays alive.
	launch func(layout *fetch.Layout, args []string) (*exec.Cmd, error)

	// demo is non-nil while the demonstration is playing. It is a fallback for
	// the absence of a phone, never an override of a present one.
	demo *demoRun
}

// StartDemo begins the demonstration and returns immediately.
//
// The timeline is driven by the ordinary poll, so nothing here spawns a
// goroutine: one clock, one place where the state is written.
func (a *App) StartDemo() {
	a.mu.Lock()
	a.demo = newDemoRun()
	log := a.log
	a.mu.Unlock()

	log.Infof("demonstração: iniciada (ciclo de %s)", demoLoop)
}

// EndDemo stops the demonstration and returns to describing real devices.
func (a *App) EndDemo() {
	a.mu.Lock()
	had := a.demo != nil
	a.demo = nil
	log := a.log
	a.mu.Unlock()

	if had {
		log.Infof("demonstração: encerrada")
	}
}

// Demoing reports whether the demonstration is currently playing.
func (a *App) Demoing() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.demo != nil
}

// New builds an App rooted at dataDir, where the scrcpy payload is installed.
//
// The language is detected once from the environment; callers that need a
// specific language can override it afterwards.
func New(dataDir string) *App {
	lang := i18n.Detect()

	a := &App{
		lang:      lang,
		ready:     make(chan struct{}),
		startedAt: time.Now(),
		state: State{
			Setup:     "installing",
			SetupMsg:  i18n.T(lang, "setup.preparing_detail"),
			ScrcpyVer: catalog.Version,
			Platform:  platformLabel(lang),
			Text:      i18n.UIStrings(lang),
			Steps:     []string{},
		},
	}
	return a
}

// SetLogger attaches a log to the App.
//
// It is separate from New so the tests can run without touching the disk, and
// so a failure to open the log is not fatal: a nil logger is valid.
func (a *App) SetLogger(l *logfile.Logger) {
	a.mu.Lock()
	a.log = l
	a.mu.Unlock()
}

// logger returns the current logger under the lock, for callers already inside
// a critical section.
func (a *App) logger() *logfile.Logger {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.log
}

// Lang returns the language in use.
func (a *App) Lang() i18n.Lang { return a.lang }

// SetLang changes the language used for subsequent state updates.
//
// The browser chrome is retranslated at the same time, because a page that
// switched the instruction but not the button label would read as a bug.
func (a *App) SetLang(l i18n.Lang) {
	a.mu.Lock()
	a.lang = l
	a.state.Platform = platformLabel(l)
	a.state.Text = i18n.UIStrings(l)
	a.mu.Unlock()
}

// platformLabel renders a human name for the running OS.
func platformLabel(lang i18n.Lang) string {
	switch runtime.GOOS {
	case "windows":
		return i18n.T(lang, "platform.windows")
	case "darwin":
		return i18n.T(lang, "platform.darwin")
	case "linux":
		return i18n.T(lang, "platform.linux")
	default:
		return runtime.GOOS
	}
}

// Install downloads, verifies and extracts scrcpy, then starts the adb daemon.
//
// It reports progress into the state so the UI can show a download bar on first
// run instead of an unexplained blank screen.
func (a *App) Install(ctx context.Context, dataDir string) error {
	log := a.logger()
	log.Infof("instalação: iniciando em %q (scrcpy %s)", dataDir, catalog.Version)

	if !catalog.Supported() {
		a.mu.RLock()
		lang := a.lang
		a.mu.RUnlock()

		msg := i18n.T(lang, "error.unsupported_platform",
			runtime.GOOS, runtime.GOARCH, catalog.SupportedList())

		log.Errorf("instalação: plataforma sem build (%s/%s)", runtime.GOOS, runtime.GOARCH)
		a.setFatal(msg)
		return fmt.Errorf("plataforma não suportada")
	}

	layout, err := fetch.Ensure(ctx, dataDir, func(s fetch.Status) {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.state.Setup = "installing"
		a.state.SetupMsg = s.Message
		a.state.SetupPct = s.Percent
	})
	if err != nil {
		log.Errorf("instalação falhou: %v", err)
		a.mu.Lock()
		a.state.Setup = "error"
		a.state.SetupMsg = a.friendlyError(err)
		a.mu.Unlock()
		return err
	}
	a.layout = layout

	log.Infof("instalação concluída: binário=%q servidor=%q", layout.Binary, layout.Server)

	adbPath, err := fetch.ResolveADB(layout)
	if err != nil {
		a.mu.Lock()
		a.state.Setup = "error"
		a.state.SetupMsg = err.Error()
		a.mu.Unlock()
		return err
	}

	a.adb = device.NewClient(adbPath)

	// Release any watcher before doing the slow daemon start, so polling can
	// begin immediately; adb start-server is best-effort and List would
	// trigger it implicitly anyway.
	a.markReady()

	// Starting the daemon is best-effort: a cold daemon is not fatal.
	if err := a.adb.StartServer(ctx); err != nil {
		log.Warnf("adb start-server falhou (o Reflexo tentará de novo): %v", err)
		a.mu.Lock()
		a.state.SetupMsg = i18n.T(a.lang, "setup.adb_starting")
		a.mu.Unlock()
	}

	a.mu.Lock()
	a.state.Setup = "ready"
	a.state.SetupMsg = i18n.T(a.lang, "setup.ready", catalog.Version)
	a.state.SetupPct = 1
	a.mu.Unlock()

	return nil
}

// friendlyError turns internal error text into something a non-technical user
// can act on.
//
// The mapping matters more than it looks: the default Go network errors are
// written for developers, and the whole premise of Reflexo is that the user
// never has to interpret one.
//
// It matches on the *internal* error text, which stays English by convention
// even in a Portuguese build, so the detection rules are not themselves
// translated.
func (a *App) friendlyError(err error) string {
	msg := err.Error()

	switch {
	case containsAny(msg, "certificate", "tls", "x509"):
		return i18n.T(a.lang, "error.certificate")
	case containsAny(msg, "no such host", "dns", "lookup"):
		return i18n.T(a.lang, "error.no_network")
	case containsAny(msg, "timeout", "deadline exceeded"):
		return i18n.T(a.lang, "error.timeout")
	case containsAny(msg, "integridade"):
		return i18n.T(a.lang, "error.integrity")
	case containsAny(msg, "access is denied", "permission denied", "permissão"):
		return i18n.T(a.lang, "error.permission")
	default:
		return msg
	}
}

// containsAny reports whether s contains any of the given substrings.
func containsAny(s string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(s, n) {
			return true
		}
	}
	return false
}

// markReady releases the watcher exactly once.
func (a *App) markReady() {
	a.readyOnce.Do(func() { close(a.ready) })
}

// Watch polls adb until the context is cancelled, refreshing State each time.
func (a *App) Watch(ctx context.Context) {
	// Block until the install has produced a usable adb. Without this the very
	// first run would start watching before adb exists, see nothing, and stop.
	//
	// The demonstration is the exception. Someone opening Reflexo for the first
	// time is, by definition, someone who has not installed anything yet, so
	// making them wait out a twenty-five megabyte download before the product
	// shows anything at all would waste the one moment they are guaranteed to
	// be watching.
	if !a.Demoing() {
		select {
		case <-ctx.Done():
			return
		case <-a.ready:
		}
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	a.Refresh(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.Refresh(ctx)
		}
	}
}

// survey is what the device list actually is right now, and where that answer
// came from.
type survey struct {
	devices []device.Device

	// demo is true when the devices are scripted rather than real.
	demo bool

	// mirroring is the demonstration's own view of whether the mirror is up.
	// It is only meaningful when demo is true.
	mirroring bool

	err error
}

// gather asks adb what is connected and decides whether that answer should be
// used.
//
// The ordering matters: a real phone outranks the demonstration. Someone who
// started the demonstration and then plugged in their phone wants their phone,
// and leaving the script running would look like a product stuck in a loop —
// the most damaging impression it could give.
func (a *App) gather(ctx context.Context) survey {
	real, err := a.listReal(ctx)

	a.mu.RLock()
	demo := a.demo
	a.mu.RUnlock()

	if demo == nil {
		return survey{devices: real, err: err}
	}

	if err == nil && len(real) > 0 {
		a.EndDemo()
		return survey{devices: real}
	}

	devices, mirroring := demo.moment()
	return survey{devices: devices, demo: true, mirroring: mirroring}
}

// Refresh performs one poll and updates the state.
func (a *App) Refresh(ctx context.Context) {
	found := a.gather(ctx)

	if found.err != nil {
		// adb itself is unreachable. This is not "no phone": it is the state
		// that used to dead-end the user, and checkADB decides whether to fix
		// it or merely report it.
		a.logDevices(nil, found.err)
		situation := a.checkADB(ctx, nil, found.err)
		a.apply(guide.Of(situation, a.lang))
		a.setDemoState(false, true, false)
		return
	}

	// adb answered, so the situation is whatever the devices say — unless the
	// list is empty moments after the mirror died, which checkADB knows to
	// distrust.
	if found.demo {
		// The script is not logged as a real device. Writing DEMO0001 into the
		// support log would put a fabricated phone next to real diagnostics,
		// and the whole value of that file is that it can be believed.
		//
		// The demo also skips checkADB entirely: adb was never involved, so
		// there is no health to judge and nothing to recover.
		a.apply(guide.Of(situationFor(found.devices), a.lang))
		a.setDemoState(true, false, found.mirroring)
		return
	}

	situation := a.checkADB(ctx, found.devices, nil)

	// Load properties for the device we are about to describe. Only the first
	// is considered: Reflexo v1 mirrors one device at a time (ADR-0004).
	if len(found.devices) == 1 && found.devices[0].State.Ready() {
		if props, err := a.adb.Props(ctx, found.devices[0].Serial); err == nil {
			found.devices[0].Props = props
		}
	}

	a.logDevices(found.devices, nil)
	a.apply(guide.Of(situation, a.lang))
	a.setDemoState(false, len(found.devices) == 0, false)
}

// listReal asks adb for the device list, tolerating adb not existing yet.
func (a *App) listReal(ctx context.Context) ([]device.Device, error) {
	if a.adb == nil {
		return nil, nil
	}
	return a.adb.List(ctx)
}

// setDemoState records whether the screen shows a demonstration, whether
// offering one would help, and what the script says about the mirror.
func (a *App) setDemoState(demoing, empty, mirroring bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.state.Demo = demoing
	a.state.CanDemo = !demoing && empty

	// The demonstration drives the mirroring screen from its own timeline, so
	// the flag has to follow the script rather than a process that was never
	// started.
	if demoing {
		a.state.Running = mirroring
	}
}

// logDevices records the device list, but only when it actually changed.
//
// Poll runs every 2 seconds. Writing every result would bury the interesting
// lines under a wall of identical entries — the single most important property
// of a support log is that the line you need is findable.
func (a *App) logDevices(devices []device.Device, listErr error) {
	var sig string
	switch {
	case listErr != nil:
		sig = "erro:" + listErr.Error()
	case len(devices) == 0:
		sig = "nenhum"
	default:
		sig = deviceSignature(devices)
	}

	a.mu.Lock()
	changed := sig != a.lastDeviceSig
	a.lastDeviceSig = sig
	log := a.log
	a.mu.Unlock()

	if !changed {
		return
	}

	switch {
	case listErr != nil:
		log.Errorf("adb devices falhou: %v", listErr)
	case len(devices) == 0:
		log.Infof("aparelhos: nenhum")
	case len(devices) == 1:
		d := devices[0]
		log.Infof("aparelho: %s estado=%s%s", d.Serial, d.State, describeProps(d))
	default:
		log.Infof("aparelhos: %d conectados (%s)",
			len(devices), deviceSignature(devices))
	}
}

// describeProps summarises what adb reported about a device, for the log only.
func describeProps(d device.Device) string {
	var out string
	if rel := d.AndroidRelease(); rel != "" {
		out += " android=" + rel
	}
	if sdk := d.SDKInt(); sdk != "" {
		out += " api=" + sdk
	}
	if m := d.DisplayName(); m != "" {
		out += " modelo=" + m
	}
	return out
}

// deviceSignature collapses a device list into one comparable string.
func deviceSignature(devices []device.Device) string {
	parts := make([]string, 0, len(devices))
	for _, d := range devices {
		parts = append(parts, string(d.State)+":"+d.Serial+describeProps(d))
	}
	return strings.Join(parts, ",")
}

// apply writes an instruction into the state without clobbering setup fields.
func (a *App) apply(in guide.Instruction) {
	a.mu.Lock()

	a.state.Severity = int(in.Severity)
	a.state.SeverityKey = in.Severity.Key()
	a.state.Headline = in.Headline
	a.state.Detail = in.Detail
	a.state.Steps = in.Steps
	a.state.CanStart = in.CanStart

	// Log the transition, not every poll. The signature includes the severity
	// and headline, so a change of advice is recorded even when the device
	// list did not change.
	sig := fmt.Sprintf("%d|%s", in.Severity, in.Headline)
	changed := sig != a.lastSig
	a.lastSig = sig

	log := a.log
	a.mu.Unlock()

	if changed {
		log.Infof("instrução: [%s] %s | pode espelhar=%v",
			severityName(in.Severity), in.Headline, in.CanStart)
	}
}

// setFatal records an unrecoverable condition; the UI shows it in place of
// everything else.
func (a *App) setFatal(msg string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state.Fatal = msg
}

// Snapshot returns a copy of the current state, safe to serialise.
func (a *App) Snapshot() State {
	a.mu.RLock()
	defer a.mu.RUnlock()

	s := a.state
	// Copy the slice and the map so a caller cannot observe a torn update, nor
	// reach into the live state through a shared backing array.
	s.Steps = append([]string(nil), a.state.Steps...)
	if a.state.Text != nil {
		s.Text = make(map[string]string, len(a.state.Text))
		for k, v := range a.state.Text {
			s.Text[k] = v
		}
	}
	return s
}

// Start launches scrcpy with the guided defaults.
//
// The whole body is serialised by startMu, including the exec of the process.
// That is not defensive overkill: the previous version read Running under a
// read lock and set it later under a write lock, so two requests arriving
// together both saw false and both launched scrcpy. Firing twelve simultaneous
// POST /api/start requests started four mirrors of the same phone, three of
// which immediately exited as scrcpy fought over the device. The user saw
// windows flashing open and shut.
//
// Locking only the flag would not have been enough either — the window has to
// stay closed until the process actually exists.
func (a *App) Start(ctx context.Context) error {
	// The demonstration has no phone to mirror, so there is nothing to launch.
	// Jumping to the mirroring step instead of returning early is what keeps the
	// button honest: a click that silently does nothing reads as a broken
	// button, which is the opposite of what the demonstration is for.
	if a.Demoing() {
		a.mu.RLock()
		demo := a.demo
		a.mu.RUnlock()

		if demo == nil {
			return fmt.Errorf("a demonstração não está ativa")
		}
		demo.showMirroring()
		a.logger().Infof("demonstração: salto para o espelhamento")
		a.Refresh(ctx)
		return nil
	}

	a.startMu.Lock()
	defer a.startMu.Unlock()

	a.mu.RLock()
	canStart := a.state.CanStart
	running := a.state.Running
	layout := a.layout
	a.mu.RUnlock()

	if layout == nil {
		a.logger().Errorf("Start recusado: scrcpy ainda não instalado")
		return fmt.Errorf("scrcpy ainda não foi instalado")
	}
	if running {
		a.logger().Warnf("Start recusado: espelhamento já está em execução")
		return fmt.Errorf("o espelhamento já está em execução")
	}
	if !canStart {
		a.logger().Warnf("Start recusado: nenhum aparelho autorizado")
		return fmt.Errorf("ainda não há aparelho autorizado para espelhar")
	}

	args := append(defaultArgs(), a.ExtraArgs...)

	launch := a.launch
	if launch == nil {
		launch = launchScrcpy
	}

	cmd, err := launch(layout, args)
	if err != nil {
		a.logger().Errorf("não foi possível montar o comando do scrcpy: %v", err)
		return fmt.Errorf("não foi possível montar o comando do scrcpy: %w", err)
	}

	if err := cmd.Start(); err != nil {
		a.logger().Errorf("scrcpy não iniciou: %v", err)
		return fmt.Errorf("não foi possível iniciar o scrcpy: %w", err)
	}

	a.mu.Lock()
	a.scrcpy = cmd
	a.state.Running = true
	log := a.log
	a.mu.Unlock()

	log.Infof("espelhamento iniciado: %q %v", layout.Binary, args)

	// Reap the process in the background so Running returns to false when the
	// user closes the mirroring window.
	go func() {
		waitErr := cmd.Wait()

		a.mu.Lock()
		a.state.Running = false
		a.mu.Unlock()

		// The exit of the mirror is the single most useful event for support:
		// a crash here is otherwise invisible, because the console that would
		// have carried scrcpy's own error is already gone.
		if waitErr != nil {
			log.Warnf("espelhamento terminou com erro: %v", waitErr)
			// Exit status 2 is scrcpy saying the device went away. Remembering
			// it is what lets the next poll tell "the phone was unplugged"
			// apart from "adb died", which produce identical output.
			a.noteMirrorLost()
		} else {
			log.Infof("espelhamento encerrado (janela fechada pelo usuário)")
		}
	}()

	return nil
}

// launchScrcpy builds the command that mirrors the device.
//
// It is a plain function rather than inline code so tests can substitute a
// process that stays alive, which is what makes the concurrent-start guard
// testable without spawning a real mirror.
func launchScrcpy(layout *fetch.Layout, args []string) (*exec.Cmd, error) {
	// #nosec G204 -- arguments are Reflexo's own constants, plus operator
	// overrides passed on our own command line.
	cmd := exec.Command(layout.Binary, args...)
	cmd.Dir = layout.Root

	// scrcpy locates scrcpy-server next to its own executable; being explicit
	// removes any dependence on the working directory.
	if layout.Server != "" {
		cmd.Env = append(os.Environ(), "SCRCPY_SERVER_PATH="+layout.Server)
	}
	if layout.IconDir != "" {
		cmd.Env = append(cmd.Env, "SCRCPY_ICON_DIR="+layout.IconDir)
	}
	if adbPath, err := fetch.ResolveADB(layout); err == nil {
		cmd.Env = append(cmd.Env, "ADB="+adbPath)
	}
	return cmd, nil
}

// defaultArgs are the options a first-time user benefits from.
//
// Deliberately conservative: --turn-screen-off is NOT enabled because turning
// the phone's screen off unannounced is surprising, and the phone may be doing
// something the user still needs to see.
func defaultArgs() []string {
	return []string{
		// Makes accented characters type correctly on the mirrored device.
		"--prefer-text",
		// Stops the phone sleeping mid-session while it is plugged in.
		"--stay-awake",
	}
}

// Stop terminates a running scrcpy, used when Reflexo shuts down.
func (a *App) Stop() {
	// A demonstration has no process to kill. Restarting the script leaves the
	// screen somewhere sensible instead of frozen on the mirroring step if
	// anything ever did call this while one was playing.
	if a.Demoing() {
		a.mu.RLock()
		demo := a.demo
		a.mu.RUnlock()
		if demo != nil {
			demo.restart()
		}
		return
	}

	a.mu.RLock()
	cmd := a.scrcpy
	log := a.log
	a.mu.RUnlock()

	if cmd != nil && cmd.Process != nil {
		log.Infof("encerrando o espelhamento (PID %d)", cmd.Process.Pid)
		_ = cmd.Process.Kill()
	}
}

// DataDir moved to paths.go: the payload location is platform policy, and it
// now needs its own module because it also carries the migration away from the
// Roaming directory the first versions used.

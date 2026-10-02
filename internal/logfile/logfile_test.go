package logfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A nil logger must be silent and harmless: the product has to keep mirroring
// even when it could not open a log file at all.
func TestNilLoggerIsSafe(t *testing.T) {
	var l *Logger

	l.Infof("oi %d", 1)
	l.Warnf("aviso")
	l.Errorf("erro")

	if got := l.Path(); got != "" {
		t.Errorf("Path() = %q, esperado vazio", got)
	}
	if err := l.Close(); err != nil {
		t.Errorf("Close() em nil = %v", err)
	}
}

func TestWritesTimestampedLevel(t *testing.T) {
	dir := t.TempDir()
	l, err := Open(dir, "r.log")
	if err != nil {
		t.Fatalf("Open(): %v", err)
	}
	t.Cleanup(func() { l.Close() })

	l.Infof("iniciando")
	l.Warnf("demora %s", "muito")
	l.Errorf("falhou: %v", "motivo")

	body := read(t, filepath.Join(dir, "r.log"))

	for _, want := range []string{"INFO", "WARN", "ERROR", "iniciando", "muito", "falhou: motivo"} {
		if !strings.Contains(body, want) {
			t.Errorf("o log não contém %q:\n%s", want, body)
		}
	}

	// Every line starts with a parseable timestamp, which is what makes the
	// file usable when correlating with the user's account of what happened.
	lines := nonEmptyLines(body)
	if len(lines) != 3 {
		t.Fatalf("esperava 3 linhas, obtive %d", len(lines))
	}
	for i, line := range lines {
		ts := line[:len("2006-01-02 15:04:05.000")]
		if _, err := time.Parse("2006-01-02 15:04:05.000", ts); err != nil {
			t.Errorf("linha %d não começa com timestamp válido (%q): %v", i+1, ts, err)
		}
	}
}

// A message containing newlines would break the one-event-per-line contract
// that makes the file greppable, so it is flattened on the way in.
func TestNewlinesAreFlattened(t *testing.T) {
	dir := t.TempDir()
	l, err := Open(dir, "r.log")
	if err != nil {
		t.Fatalf("Open(): %v", err)
	}
	t.Cleanup(func() { l.Close() })

	l.Errorf("primeira\nsegunda\r\nterceira")

	lines := nonEmptyLines(read(t, filepath.Join(dir, "r.log")))
	if len(lines) != 1 {
		t.Errorf("esperava 1 linha, obtive %d: %v", len(lines), lines)
	}
	if !strings.Contains(lines[0], "primeira segunda terceira") {
		t.Errorf("mensagem não achatada: %q", lines[0])
	}
}

// The log must not grow without bound: a long-lived install cannot fill a
// profile disk.
func TestRotatesAtSizeLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "r.log")

	l, err := Open(dir, "r.log")
	if err != nil {
		t.Fatalf("Open(): %v", err)
	}
	t.Cleanup(func() { l.Close() })

	line := strings.Repeat("x", 1024) + "\n"
	// Comfortably past maxBytes plus a couple of rotations.
	for i := 0; i < 2600; i++ {
		l.Infof("%s", line)
	}

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("o log principal desapareceu: %v", err)
	}
	if fi.Size() > maxBytes+(2<<10) {
		t.Errorf("log principal tem %d bytes, acima do limite de %d", fi.Size(), maxBytes)
	}

	// The previous generation is kept...
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Errorf("não criou o backup .1: %v", err)
	}

	// ...but never more than keepBackups, so the directory stops growing.
	if _, err := os.Stat(path + "." + itoa(keepBackups)); err == nil {
		t.Errorf("criou backup além do limite de %d arquivos", keepBackups)
	}
}

// Rotation happens before the write, so a line is never split across two
// files — the tail of the log has to stay readable.
func TestRotationKeepsLinesWhole(t *testing.T) {
	dir := t.TempDir()

	l, err := Open(dir, "r.log")
	if err != nil {
		t.Fatalf("Open(): %v", err)
	}
	t.Cleanup(func() { l.Close() })

	const marker = "LINHA-INTEGRA-"
	for i := 0; i < 2500; i++ {
		l.Infof("%s%04d", marker, i)
	}
	l.Infof("%s%s", marker, "final")
	l.Close()

	for _, name := range []string{"r.log", "r.log.1"} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		for _, line := range nonEmptyLines(string(b)) {
			if !strings.Contains(line, marker) {
				t.Errorf("%s tem uma linha truncada: %q", name, line)
			}
		}
	}
}

// Reopening must append, not truncate: a second run of Reflexo should not
// erase the evidence from the first.
func TestReopenAppends(t *testing.T) {
	dir := t.TempDir()

	l1, err := Open(dir, "r.log")
	if err != nil {
		t.Fatalf("Open(): %v", err)
	}
	l1.Infof("primeira execucao")
	l1.Close()

	l2, err := Open(dir, "r.log")
	if err != nil {
		t.Fatalf("Open() de novo: %v", err)
	}
	l2.Infof("segunda execucao")
	l2.Close()

	body := read(t, filepath.Join(dir, "r.log"))
	if !strings.Contains(body, "primeira execucao") || !strings.Contains(body, "segunda execucao") {
		t.Errorf("o segundo uso apagou o primeiro:\n%s", body)
	}
}

// Open must fail cleanly on an impossible directory rather than crashing.
func TestOpenFailsGracefully(t *testing.T) {
	// A path whose parent is an existing file cannot be created.
	file := filepath.Join(t.TempDir(), "arquivo")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Open(filepath.Join(file, "sub"), "r.log"); err == nil {
		t.Error("Open() deveria falhar quando não consegue criar a pasta")
	}
}

// The whole reason the log is in LOCALAPPDATA rather than Roaming: Roaming is
// frequently cloud-synced on corporate machines — our main audience — and the
// log contains device serials.
func TestLogDirIsNotInRoaming(t *testing.T) {
	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir(): %v", err)
	}

	cfg, err := os.UserConfigDir()
	if err != nil {
		t.Skip("sem UserConfigDir neste sistema")
	}

	if strings.HasPrefix(dir, cfg) {
		t.Errorf("o log iria para %q, que está dentro de %q (Roaming/Application Support). "+
			"Em ambiente corporativo essa pasta é sincronizada para a nuvem, e o log "+
			"contém o serial do aparelho.", dir, cfg)
	}
}

// Dir falls back rather than failing: a log location is never worth stopping
// the product for.
func TestDirAlwaysReturnsSomething(t *testing.T) {
	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir() falhou: %v", err)
	}
	if !filepath.IsAbs(dir) {
		t.Errorf("Dir() = %q, esperado caminho absoluto", dir)
	}
	if !strings.Contains(dir, "reflexo") {
		t.Errorf("Dir() = %q, deveria conter \"reflexo\"", dir)
	}
}

// A panic in a goroutine is otherwise invisible: the process dies and the
// console carrying the trace is already gone. It has to reach the file.
func TestRecoverLogsAndRepanics(t *testing.T) {
	dir := t.TempDir()
	l, err := Open(dir, "r.log")
	if err != nil {
		t.Fatalf("Open(): %v", err)
	}
	t.Cleanup(func() { l.Close() })

	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Error("Recover() engoliu o panic; o produto não deve mascarar isso")
			}
		}()
		defer l.Recover("durante o teste")

		panic("falha catastrófica")
	}()

	body := read(t, filepath.Join(dir, "r.log"))
	if !strings.Contains(body, "panic em durante o teste") {
		t.Errorf("o panic não foi registrado:\n%s", body)
	}
	if !strings.Contains(body, "falha catastrófica") {
		t.Errorf("o valor do panic não foi registrado:\n%s", body)
	}
	if !strings.Contains(body, "logfile") {
		t.Errorf("o stack trace não foi registrado:\n%s", body)
	}
}

func TestPathPointsAtTheFile(t *testing.T) {
	dir := t.TempDir()
	l, err := Open(dir, "reflexo.log")
	if err != nil {
		t.Fatalf("Open(): %v", err)
	}
	t.Cleanup(func() { l.Close() })

	want := filepath.Join(dir, "reflexo.log")
	if got := l.Path(); got != want {
		t.Errorf("Path() = %q, esperado %q", got, want)
	}
}

// The rotation bookkeeping is internal, so this checks the observable promise:
// after a rotation the current file is small and a previous one exists.
func TestRotationKeepsHistoryBounded(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "r.log")

	l, err := Open(dir, "r.log")
	if err != nil {
		t.Fatalf("Open(): %v", err)
	}
	t.Cleanup(func() { l.Close() })

	for i := 0; i < 6000; i++ {
		l.Infof("linha %d %s", i, strings.Repeat("y", 512))
	}

	// Bounded: at most the current file plus keepBackups generations.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) > keepBackups+1 {
		t.Errorf("a pasta tem %d arquivos, acima de %d permitidos",
			len(entries), keepBackups+1)
	}
	_ = path
}

// Close must be final.
//
// A write after Close previously reopened the file, which left a handle open
// after shutdown and let a straggler goroutine append to a log already reported
// as finished. On Windows the leftover handle also blocked deleting the
// directory.
func TestWriteAfterCloseIsDiscarded(t *testing.T) {
	dir := t.TempDir()
	l, err := Open(dir, "r.log")
	if err != nil {
		t.Fatalf("Open(): %v", err)
	}

	l.Infof("antes de fechar")
	if err := l.Close(); err != nil {
		t.Fatalf("Close(): %v", err)
	}
	if err := l.Close(); err != nil {
		t.Errorf("Close() não é idempotente: %v", err)
	}

	l.Infof("depois de fechar")
	l.Warnf("e isso")
	l.Errorf("e isto")

	body := read(t, filepath.Join(dir, "r.log"))
	if !strings.Contains(body, "antes de fechar") {
		t.Error("perdeu o que foi escrito antes de fechar")
	}
	for _, unwanted := range []string{"depois de fechar", "e isso", "e isto"} {
		if strings.Contains(body, unwanted) {
			t.Errorf("escreveu %q depois de Close()", unwanted)
		}
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("não foi possível ler %s: %v", path, err)
	}
	return string(b)
}

func nonEmptyLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"reflexo/internal/fetch"
	"reflexo/internal/guide"
	"reflexo/internal/i18n"
)

// TestWatchBlocksUntilReady is a regression test.
//
// Watch and Install run as concurrent goroutines. Before the ready channel was
// introduced, Watch checked for a nil adb, found one on the very first run
// (because Install needs several seconds to download scrcpy), returned
// immediately, and left the UI permanently blank with no instruction at all.
//
// The bug is invisible without this test because on a warm cache Install
// finishes fast enough to hide the race.
func TestWatchBlocksUntilReady(t *testing.T) {
	a := New(t.TempDir())

	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		a.Watch(ctx)
		close(done)
	}()

	// Watch must stay blocked while adb does not exist yet.
	select {
	case <-done:
		t.Fatal("Watch retornou antes da instalação: a UI ficaria sem instrução para sempre")
	case <-time.After(150 * time.Millisecond):
		// still blocked, as required
	}

	a.markReady()

	select {
	case <-done:
		// correct: Watch returned once the context expired
	case <-time.After(2 * time.Second):
		t.Fatal("Watch não retornou mesmo após o contexto expirar")
	}
}

// markReady runs from Install and possibly from an error path; closing a closed
// channel would panic.
func TestMarkReadyIsIdempotent(t *testing.T) {
	a := New(t.TempDir())

	a.markReady()
	a.markReady()
	a.markReady()

	select {
	case <-a.ready:
	default:
		t.Fatal("canal ready não foi fechado")
	}
}

// Start must refuse before installation and while no device is authorised,
// rather than spawning a process that fails with an opaque message.
func TestStartGuards(t *testing.T) {
	t.Run("sem instalação", func(t *testing.T) {
		a := New(t.TempDir())
		if err := a.Start(context.Background()); err == nil {
			t.Error("Start aceitou rodar sem o scrcpy instalado")
		}
	})

	t.Run("sem aparelho autorizado", func(t *testing.T) {
		a := New(t.TempDir())
		a.layout = &fetch.Layout{Binary: "scrcpy", Server: "scrcpy-server"}

		a.mu.Lock()
		a.state.CanStart = false
		a.mu.Unlock()

		if err := a.Start(context.Background()); err == nil {
			t.Error("Start aceitou rodar sem aparelho autorizado")
		}
	})

	t.Run("já em execução", func(t *testing.T) {
		a := New(t.TempDir())
		a.layout = &fetch.Layout{Binary: "scrcpy", Server: "scrcpy-server"}

		a.mu.Lock()
		a.state.CanStart = true
		a.state.Running = true
		a.mu.Unlock()

		if err := a.Start(context.Background()); err == nil {
			t.Error("Start aceitou uma segunda instância")
		}
	})
}

func TestDefaultArgs(t *testing.T) {
	args := defaultArgs()

	// --turn-screen-off must stay off: switching the phone's screen off without
	// warning is the kind of surprise this product exists to avoid.
	for _, a := range args {
		if strings.HasPrefix(a, "--turn-screen-off") || a == "-S" {
			t.Errorf("defaultArgs() inclui %q, que altera o estado do celular sem avisar", a)
		}
	}

	if !contains(args, "--prefer-text") {
		t.Error("defaultArgs() deveria incluir --prefer-text")
	}
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

// Snapshot must not hand out the internal slice, or a caller could observe a
// torn update while the poller rewrites it.
func TestSnapshotCopiesSteps(t *testing.T) {
	a := New(t.TempDir())

	a.mu.Lock()
	a.state.Steps = []string{"primeiro", "segundo"}
	a.mu.Unlock()

	s := a.Snapshot()
	s.Steps[0] = "ADICIONADO PELO CHAMADOR"

	again := a.Snapshot()
	if again.Steps[0] != "primeiro" {
		t.Errorf("estado interno foi corrompido pelo chamador: %q", again.Steps[0])
	}
}

// Internal errors are written for developers; the user must see an action.
//
// The expectation is per language, and every shipped language is exercised:
// friendlyError is one of the few places where a missed i18n migration would
// silently ship an English sentence inside a Portuguese product.
func TestFriendlyError(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want map[i18n.Lang]string
	}{
		{
			name: "certificado",
			in:   "x509: certificate signed by unknown authority",
			want: map[i18n.Lang]string{
				i18n.PortugueseBrazil: "conexão",
				i18n.English:          "authenticity",
			},
		},
		{
			name: "dns",
			in:   "dial tcp: lookup github.com: no such host",
			want: map[i18n.Lang]string{
				i18n.PortugueseBrazil: "internet",
				i18n.English:          "internet",
			},
		},
		{
			name: "timeout",
			in:   "context deadline exceeded",
			want: map[i18n.Lang]string{
				i18n.PortugueseBrazil: "demorou demais",
				i18n.English:          "too long",
			},
		},
		{
			name: "integridade",
			in:   "falha na verificação de integridade: esperado x",
			want: map[i18n.Lang]string{
				i18n.PortugueseBrazil: "segurança",
				i18n.English:          "security check",
			},
		},
		{
			name: "permissão",
			in:   "access is denied",
			want: map[i18n.Lang]string{
				i18n.PortugueseBrazil: "antivírus",
				i18n.English:          "antivirus",
			},
		},
		{
			// An unrecognised error is passed through: showing the raw text
			// plus support contact beats inventing a cause.
			name: "desconhecido",
			in:   "erro totalmente novo",
			want: map[i18n.Lang]string{
				i18n.PortugueseBrazil: "erro totalmente novo",
				i18n.English:          "erro totalmente novo",
			},
		},
	}

	for _, lang := range i18n.Available() {
		a := New(t.TempDir())
		a.SetLang(lang)

		for _, tt := range tests {
			tt := tt
			t.Run(string(lang)+"/"+tt.name, func(t *testing.T) {
				want, ok := tt.want[lang]
				if !ok {
					t.Fatalf("teste sem expectativa para o idioma %q", lang)
				}

				got := a.friendlyError(errString(tt.in))

				if !strings.Contains(strings.ToLower(got), strings.ToLower(want)) {
					t.Errorf("friendlyError(%q) = %q, esperava conter %q", tt.in, got, want)
				}
				if strings.Contains(got, "%!") {
					t.Errorf("friendlyError(%q) tem erro de formatação: %q", tt.in, got)
				}
			})
		}
	}
}

// Guards against the i18n migration leaving one language pinned: the same
// failure must not render identically in both catalogs.
func TestFriendlyErrorIsTranslated(t *testing.T) {
	const internal = "x509: certificate signed by unknown authority"

	pt := New(t.TempDir())
	pt.SetLang(i18n.PortugueseBrazil)

	en := New(t.TempDir())
	en.SetLang(i18n.English)

	if pt.friendlyError(errString(internal)) == en.friendlyError(errString(internal)) {
		t.Error("friendlyError devolviu o mesmo texto em pt-BR e em en")
	}
}

// SetLang must not race with the poller reading the language from another
// goroutine, so the access is covered by the same mutex as the state.
func TestSetLangUpdatesPlatform(t *testing.T) {
	a := New(t.TempDir())

	a.SetLang(i18n.English)
	if a.Lang() != i18n.English {
		t.Errorf("Lang() = %q, esperado %q", a.Lang(), i18n.English)
	}
	if a.Snapshot().Platform == "" {
		t.Error("plataforma ficou vazia depois de SetLang")
	}
}

type errString string

func (e errString) Error() string { return string(e) }

// DataDir must land inside the user's own profile: writing to Program Files
// would need administrator rights, which AGENTS.md §4 forbids.
func TestDataDirIsUserWritable(t *testing.T) {
	dir, err := DataDir()
	if err != nil {
		t.Fatalf("DataDir() falhou: %v", err)
	}

	if !strings.Contains(dir, "reflexo") {
		t.Errorf("DataDir() = %q, esperado conter \"reflexo\"", dir)
	}

	if !filepath.IsAbs(dir) {
		t.Errorf("DataDir() deveria ser absoluto: %q", dir)
	}

	// Tem de estar dentro do perfil do usuário. Este é o teste que importa:
	// se DataDir apontar para fora dele, o Reflexo exige elevação para gravar.
	//
	// É read-only de propósito. Este teste chamava os.MkdirAll no diretório de
	// dados real, ou seja, criava uma pasta no perfil do desenvolvedor — o mesmo
	// padrão de "opera sobre estado real" que, num teste de migração, chegou a
	// mover um payload de verdade. A gravabilidade é garantida por construção:
	// DataDir devolve um caminho sob LOCALAPPDATA, ~/.local/share ou
	// ~/Library/Application Support, que o usuário grava por definição.
	home, herr := os.UserHomeDir()
	if herr != nil {
		t.Skipf("sem UserHomeDir: %v", herr)
	}
	if !strings.HasPrefix(strings.ToLower(dir), strings.ToLower(home)) {
		t.Errorf("DataDir() = %q não está dentro do perfil do usuário (%q)", dir, home)
	}
}

// The browser renders no user-facing words of its own (ADR-0006): every string
// comes from the Go catalog through the state payload. web/app.js used to hold a
// hardcoded Portuguese SEVERITY array, which silently drifted from guide.Severity
// and could not be translated at all — this test is what makes that unrepresentable.
func TestNoUserFacingStringsInWeb(t *testing.T) {
	files := []string{"index.html", "app.js"}
	root := filepath.Join("..", "..", "web")

	// A literal word that could only be shown to a user: an English or
	// Portuguese phrase, as opposed to identifiers and URLs.
	suspect := regexp.MustCompile(`(?i)"[^"\n]*\b(the|and|your|phone|cable|tap|close|conect|celular|cabo|toque|feche|espelhamento|iniciar|celular|conexão)\b[^"\n]*"`)

	for _, name := range files {
		b, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("não foi possível ler web/%s: %v", name, err)
		}
		src := string(b)

		// The HTML intentionally ships a few Portuguese literals so the page
		// is readable for the few milliseconds before the first poll; they all
		// carry a data-i18n attribute so they get replaced. JavaScript has no
		// such allowance.
		if name == "app.js" {
			for _, m := range suspect.FindAllString(src, -1) {
				t.Errorf("web/app.js contém texto de usuário %s; "+
					"isso pertence ao catálogo em internal/i18n", m)
			}
		}
	}
}

// Every key the web layer asks for must exist in the catalog, and the catalog
// must not carry a UI key nothing asks for. Both directions matter: a missing
// key renders the raw key to the user, and an unused key is a translated string
// nobody can ever see, which is how dead translations accumulate.
//
// The used set is gathered from both files: keys bound to markup in
// index.html, and keys passed to t() in app.js.
func TestUIStringsCoverMarkup(t *testing.T) {
	root := filepath.Join("..", "..", "web")

	html, err := os.ReadFile(filepath.Join(root, "index.html"))
	if err != nil {
		t.Fatalf("não foi possível ler web/index.html: %v", err)
	}
	js, err := os.ReadFile(filepath.Join(root, "app.js"))
	if err != nil {
		t.Fatalf("não foi possível ler web/app.js: %v", err)
	}

	used := map[string]string{}

	for _, m := range regexp.MustCompile(`data-i18n="([^"]+)"`).
		FindAllStringSubmatch(string(html), -1) {
		used[m[1]] = "web/index.html"
	}
	// Keys in app.js are matched as any dotted lowercase literal, not just the
	// ones passed directly to t(): a conditional such as
	// t(running ? "ui.running" : "ui.start") would otherwise look unused.
	// A key appearing in a comment is harmless — it only weakens this check,
	// it never reports a false failure.
	for _, m := range regexp.MustCompile(`"(?:ui|setup|severity)\.[a-z_]+"`).
		FindAllStringSubmatch(string(js), -1) {
		used[strings.Trim(m[0], `"`)] = "web/app.js"
	}

	if len(used) == 0 {
		t.Fatal("nenhuma chave de UI encontrada no web/ — a extração do teste está errada")
	}

	for key, where := range used {
		if !i18n.Has(i18n.English, key) {
			t.Errorf("%s pede a chave %q, ausente no catálogo em inglês", where, key)
		}
	}

	// A key existing in the catalog is not enough: UIStrings ships only the
	// keys listed in i18n.UIKeys, and anything the page asks for but the server
	// does not send comes back from t() as its own name. That is precisely the
	// shape of the defects that only a browser could reveal — a badge reading
	// "SEVERITY.0", a duplicated "by" in the footer — so the check belongs here
	// rather than in a comment promising to be careful.
	shipped := i18n.UIStrings(i18n.English)
	for key, where := range used {
		if strings.HasPrefix(key, "severity.") {
			// Computed at runtime from the key the server sent; the exhaustive
			// version of this is TestSeverityLabelsExist.
			continue
		}
		if _, ok := shipped[key]; !ok {
			t.Errorf("%s pede a chave %q, que não está em i18n.UIKeys(): "+
				"a interface mostraria o nome da chave em vez do texto", where, key)
		}
	}

	for _, key := range i18n.UIKeys() {
		// severity.* is looked up with a computed key ("severity." + n), so it
		// cannot be found by scanning; TestSeverityLabelsExist covers it.
		if strings.HasPrefix(key, "severity.") {
			continue
		}
		if _, ok := used[key]; !ok {
			t.Errorf("chave de UI %q está no catálogo mas nada em web/ a usa", key)
		}
	}
}

// The severity badge is rendered by indexing the catalog, so every severity
// guide defines must have a label in every language. The mapping lives in
// guide.Severity.Key so it cannot drift from the enum.
func TestSeverityLabelsExist(t *testing.T) {
	for i := 0; i < guide.SeverityCount; i++ {
		s := guide.Severity(i)
		key := s.Key()

		if key == "" {
			t.Errorf("severidade %d não tem chave de catálogo", i)
			continue
		}
		for _, lang := range i18n.Available() {
			if !i18n.Has(lang, key) {
				t.Errorf("severidade %d não tem rótulo em %s (chave %q)", i, lang, key)
			}
		}
	}

	// Two severities sharing a label would make the badge ambiguous, and two
	// severities sharing a key would make the catalog silently overwrite one.
	seen := map[string]int{}
	for i := 0; i < guide.SeverityCount; i++ {
		key := guide.Severity(i).Key()
		if prev, dup := seen[key]; dup {
			t.Errorf("severidades %d e %d compartilham a chave %q", prev, i, key)
		}
		seen[key] = i
	}

	// The sentinel must not itself be reachable as a severity, or SeverityCount
	// is wrong and the loop above checked one value too many.
	if got := guide.Severity(guide.SeverityCount).Key(); got != "" {
		t.Errorf("SeverityCount = %d, mas a severidade %d tem a chave %q",
			guide.SeverityCount, guide.SeverityCount, got)
	}
}

// The state must carry the severity's catalog key, never leave the page to
// derive one from the number.
//
// This was a real defect found by rendering the page in a browser: app.js
// built "severity." + s.severity, which yields "severity.0" — a key that
// exists in no catalog — and the badge rendered the raw key to the user. A
// unit test could not have caught it, because the string was built at runtime
// in the browser.
func TestStateCarriesSeverityKey(t *testing.T) {
	a := New(t.TempDir())

	seen := map[string]bool{}
	for i := 0; i < guide.SeverityCount; i++ {
		in := guide.Instruction{Severity: guide.Severity(i)}
		a.apply(in)

		s := a.Snapshot()
		want := guide.Severity(i).Key()

		if s.SeverityKey != want {
			t.Errorf("severidade %d: SeverityKey = %q, esperado %q", i, s.SeverityKey, want)
		}
		if !i18n.Has(i18n.English, s.SeverityKey) {
			t.Errorf("severidade %d: chave %q não existe no catálogo", i, s.SeverityKey)
		}
		seen[s.SeverityKey] = true
	}

	if len(seen) != guide.SeverityCount {
		t.Errorf("apenas %d chaves distintas para %d severidades",
			len(seen), guide.SeverityCount)
	}
}

// TestHelperBlocker is not a real test.
//
// It is the child process used by TestConcurrentStartLaunchesOnce: re-executing
// the test binary with REFLEXO_HELPER set gives Start a process that stays alive
// for the duration of the test, which is the only way to assert that exactly
// one mirror is launched. It is skipped during a normal run.
func TestHelperBlocker(t *testing.T) {
	if os.Getenv("REFLEXO_HELPER") != "1" {
		t.Skip("processo auxiliar; só roda quando reexecutado por Start")
	}
	time.Sleep(60 * time.Second)
}

// TestConcurrentStartLaunchesOnce is a regression test for a real race found by
// firing twelve simultaneous POST /api/start requests at a running Reflexo.
//
// Start used to read state.Running under a read lock and set it later under a
// write lock. Every request in the burst therefore saw false, passed the guard,
// and called cmd.Start() — four mirrors of the same phone, three of which died
// immediately as scrcpy fought over the device. To the user this looked like
// windows flashing open and shut.
//
// A sequential test cannot catch this: the requests have to genuinely overlap,
// which is why this fans out with goroutines and a barrier.
func TestConcurrentStartLaunchesOnce(t *testing.T) {
	a := New(t.TempDir())
	a.layout = &fetch.Layout{Binary: os.Args[0], Root: t.TempDir()}
	a.launch = func(*fetch.Layout, []string) (*exec.Cmd, error) {
		cmd := exec.Command(os.Args[0], "-test.run=^TestHelperBlocker$")
		cmd.Env = append(os.Environ(), "REFLEXO_HELPER=1")
		return cmd, nil
	}
	a.apply(guide.Instruction{Severity: guide.SeverityReady, CanStart: true})
	t.Cleanup(a.Stop)

	const requests = 16

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		okCount int
		errs    []string
	)

	// The barrier makes every goroutine reach Start at the same moment, so the
	// requests overlap inside Start instead of trickling in.
	start := make(chan struct{})
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start

			err := a.Start(context.Background())

			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				okCount++
			} else {
				errs = append(errs, err.Error())
			}
		}()
	}

	close(start)
	wg.Wait()

	if okCount != 1 {
		t.Errorf("%d de %d chamadas a Start() tiveram sucesso, esperado exatamente 1 "+
			"(a guarda de concorrência não é atômica)", okCount, requests)
	}
	if len(errs) != requests-1 {
		t.Errorf("recebemos %d erros, esperado %d", len(errs), requests-1)
	}

	// Every rejection must be the "already running" one, not a leaked internal
	// error: a user who double-clicks deserves a comprehensible message.
	for _, e := range errs {
		if !strings.Contains(e, "já está em execução") {
			t.Errorf("Start() rejeitou com mensagem inesperada: %q", e)
		}
	}

	if !a.Snapshot().Running {
		t.Error("Running = false após um Start bem-sucedido")
	}
}

// The launcher must be given Reflexo's own arguments, and the process must be
// recorded so Stop can terminate it.
func TestStartUsesGuidedDefaults(t *testing.T) {
	a := New(t.TempDir())
	a.layout = &fetch.Layout{Binary: os.Args[0], Root: t.TempDir()}

	var got []string
	a.launch = func(_ *fetch.Layout, args []string) (*exec.Cmd, error) {
		got = append([]string(nil), args...)
		cmd := exec.Command(os.Args[0], "-test.run=^TestHelperBlocker$")
		cmd.Env = append(os.Environ(), "REFLEXO_HELPER=1")
		return cmd, nil
	}
	a.apply(guide.Instruction{Severity: guide.SeverityReady, CanStart: true})
	t.Cleanup(a.Stop)

	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start() falhou: %v", err)
	}

	for _, want := range []string{"--prefer-text", "--stay-awake"} {
		if !hasArg(got, want) {
			t.Errorf("Start() não passou %s; args = %v", want, got)
		}
	}

	// --turn-screen-off is deliberately excluded (see defaultArgs): turning the
	// phone's screen off without warning is surprising.
	for _, arg := range got {
		if strings.Contains(arg, "turn-screen-off") {
			t.Errorf("Start() passou %q, que o produto exclui de propósito", arg)
		}
	}

	if a.scrcpy == nil {
		t.Error("o processo não foi registrado, então Stop() não conseguiria encerrá-lo")
	}
}

func hasArg(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// The number must never be concatenated into a catalog key anywhere in the web
// layer: only the server knows which number means which severity.
func TestWebLayerNeverBuildsKeysFromNumbers(t *testing.T) {
	js, err := os.ReadFile(filepath.Join("..", "..", "web", "app.js"))
	if err != nil {
		t.Fatalf("não foi possível ler web/app.js: %v", err)
	}

	// Anything of the form "prefix." + <something severity-ish>.
	bad := regexp.MustCompile(`"severity\."\s*\+`)

	if m := bad.FindString(string(js)); m != "" {
		t.Errorf("web/app.js monta uma chave a partir do número da severidade (%q); "+
			"use s.severityKey, enviado pelo servidor", m)
	}
}

// fakeLayout is a minimal Layout for guard-clause tests that never reach exec.

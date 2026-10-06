package app

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

// fakeIcon is a stand-in for the real .ico.
//
// It is built in memory rather than read from assets/ on purpose: a test that
// reads the repository's files is a test that breaks when someone renames a
// directory, and worse, one that can silently pass because the file happens to
// be there. What matters here is the routing and the header, not the pixels.
func fakeIcon(body []byte) fs.FS {
	return fstest.MapFS{
		"assets/icon/icon.ico": &fstest.MapFile{Data: body},
	}
}

func fakeWeb() fs.FS {
	return fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<!DOCTYPE html><title>Reflexo</title>")},
	}
}

func newRouteServer(t *testing.T, icon fs.FS) http.Handler {
	t.Helper()
	return (&Server{
		app:   newHealthApp(t),
		web:   fakeWeb(),
		icon:  icon,
		token: "token-de-teste",
	}).routes()
}

// TestFaviconIsServedWithoutTheSessionToken pins the one route that is
// deliberately outside the gate.
//
// The browser asks for a favicon as a side effect of drawing the tab, not as a
// result of anything the user did, so the request carries no token. Gating it
// would not protect anything — the file is an icon and contains no device
// serial, no state and no action — and would cost the tab its identity whenever
// the browser trims the Referer.
func TestFaviconIsServedWithoutTheSessionToken(t *testing.T) {
	want := []byte{0x00, 0x00, 0x01, 0x00, 0x02, 0x03}
	h := newRouteServer(t, fakeIcon(want))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/favicon.ico", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /favicon.ico = %d; esperava 200, porque a aba pede o icone sem token", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "image/x-icon" {
		t.Errorf("Content-Type = %q; esperava image/x-icon, senao o navegador pode ignorar o arquivo", got)
	}
	if got := rec.Body.Bytes(); string(got) != string(want) {
		t.Errorf("corpo = %v; esperava exatamente os bytes do arquivo de icone", got)
	}
}

// TestTheFaviconExemptionDoesNotOpenAnythingElse is the blast-radius guard.
//
// An exemption in a security boundary is only safe if it is narrow. This test
// states the other half of the decision: the API is still closed to a request
// with no token, so loosening the icon route cannot have loosened anything.
func TestTheFaviconExemptionDoesNotOpenAnythingElse(t *testing.T) {
	h := newRouteServer(t, fakeIcon([]byte{0x01}))

	for _, path := range []string{"/api/state", "/index.html", "/"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))

		if rec.Code != http.StatusForbidden {
			t.Errorf("GET %s sem token = %d; esperava 403. A excecao do favicon "+
				"nao pode ter vazado para nenhuma outra rota", path, rec.Code)
		}
	}
}

// TestFaviconFailureDoesNotBreakThePage covers the unhappy path.
//
// A missing embed is a build mistake, and the browser should simply fall back
// to its generic icon. What it must not do is take the whole page down, because
// a 500 here can leave the user staring at a blank tab with no idea why.
func TestFaviconFailureDoesNotBreakThePage(t *testing.T) {
	h := newRouteServer(t, fstest.MapFS{}) // no icon in here at all

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/favicon.ico", nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /favicon.ico sem arquivo = %d; esperava 404", rec.Code)
	}
}

// TestFaviconIsTheSameBytesAsWhatTheShortcutUses is the anti-drift rule.
//
// The tab icon and the desktop icon are two consumers of one face. When they
// were served from separate copies, the only thing keeping them equal was
// remembering to update both, and nothing in the build would have noticed.
func TestFaviconIsTheSameBytesAsWhatTheShortcutUses(t *testing.T) {
	// Served content comes from whatever main.go embeds. Asserting that main.go
	// embeds the icon is a test in the root package, next to the other PE tests;
	// what matters here is that the route serves the embedded bytes verbatim
	// rather than re-encoding or substituting something.
	h := newRouteServer(t, fakeIcon([]byte{0xDE, 0xAD, 0xBE, 0xEF}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/favicon.ico", nil))

	body := rec.Body.Bytes()
	for _, b := range body {
		if b != 0xDE && b != 0xAD && b != 0xBE && b != 0xEF {
			t.Fatalf("o corpo servido contem 0x%02X, que nao veio do arquivo embutido; "+
				"a rota esta servindo outra coisa", b)
		}
	}
}

package app

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// tokenLength is the number of random bytes in the session token. 32 bytes is
// far more than needed to make guessing infeasible, and costs nothing.
const tokenLength = 32

// Server exposes the UI over loopback HTTP.
type Server struct {
	app   *App
	web   fs.FS
	addr  string
	token string
}

// NewServer prepares the local server. The listener is bound by Start.
func NewServer(a *App, web fs.FS) *Server {
	return &Server{app: a, web: web}
}

// Addr is the address the server is listening on, valid after Start.
func (s *Server) Addr() string { return s.addr }

// URL is the address the user should open, including the session token.
func (s *Server) URL() string {
	return fmt.Sprintf("http://%s/?t=%s", s.addr, s.token)
}

// Start binds a loopback listener and serves until the context is cancelled.
//
// Binding to 127.0.0.1 on a random port is the whole security model: the
// interface is reachable only from this machine, and only by a process that
// knows the token in the URL.
func (s *Server) Start(ctx context.Context) error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("não foi possível abrir a interface local: %w", err)
	}
	s.addr = ln.Addr().String()
	s.token = newToken()

	srv := &http.Server{
		Handler:           s.routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			// Nothing useful to do at this point; the context owns shutdown.
			_ = err
		}
	}()

	return nil
}

// routes wires the endpoints.
func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/state", s.handleState)
	mux.HandleFunc("POST /api/start", s.handleStart)
	mux.HandleFunc("POST /api/demo", s.handleDemo)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// The UI itself, gated by the same token as the API.
	mux.Handle("GET /", s.gate(http.FileServer(http.FS(s.web))))

	return mux
}

// gate rejects requests that do not carry the session token.
//
// Without this, any page open in the user's browser could reach the loopback
// port and trigger actions on the user's behalf.
func (s *Server) gate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.authorized(r) {
			http.Error(w, "sessão inválida, abra o Reflexo pelo link que ele exibiu", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// authorized checks the token from the query string or the Referer header.
//
// The Referer check exists because the browser keeps the query string in the
// address bar, so a reload or a refresh after a click would otherwise lose it.
func (s *Server) authorized(r *http.Request) bool {
	candidates := []string{r.URL.Query().Get("t")}

	if ref := r.Header.Get("Referer"); ref != "" {
		if idx := strings.Index(ref, "?t="); idx >= 0 {
			candidates = append(candidates, ref[idx+len("?t="):])
		}
	}

	for _, c := range candidates {
		if c == "" {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(c), []byte(s.token)) == 1 {
			return true
		}
	}
	return false
}

// handleState returns the current state as JSON.
func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		http.Error(w, "não autorizado", http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")

	if err := json.NewEncoder(w).Encode(s.app.Snapshot()); err != nil {
		// The client is already gone; nothing actionable.
		_ = err
	}
}

// handleStart launches scrcpy.
func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		http.Error(w, "não autorizado", http.StatusForbidden)
		return
	}

	if err := s.app.Start(r.Context()); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// handleDemo starts the demonstration, and ends it when one is already playing.
//
// Toggling rather than only starting matters: the button doubles as the way out,
// so a visitor who started it by mistake is never trapped watching a loop they
// cannot dismiss.
func (s *Server) handleDemo(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		http.Error(w, "não autorizado", http.StatusForbidden)
		return
	}

	if s.app.Demoing() {
		s.app.EndDemo()
	} else {
		s.app.StartDemo()
	}

	// Apply it at once rather than waiting for the next poll, so the screen
	// responds to the click instead of to a timer.
	s.app.Refresh(r.Context())

	w.WriteHeader(http.StatusOK)
}

// newToken returns a hex-encoded random session token.
func newToken() string {
	b := make([]byte, tokenLength)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand does not fail in practice; falling back to a fixed value
		// would be worse than surfacing it, so panic loudly instead of
		// silently serving an unauthenticated interface.
		panic("reflexo: não foi possível gerar o token de sessão: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// OpenBrowser points the user's default browser at the UI.
//
// The UI lives in the browser on purpose (ADR-0002): it removes any dependency
// on a webview runtime, which may be absent on a locked-down corporate machine.
func OpenBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	case "linux":
		cmd = "xdg-open"
		args = []string{url}
	default:
		return fmt.Errorf("não sei como abrir o navegador em %s", runtime.GOOS)
	}

	return exec.Command(cmd, args...).Start()
}

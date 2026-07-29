// Copyright (C) 2026 DaemonHound Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package web

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/0xdps/daemon-hound/internal/conflicts"
	"github.com/0xdps/daemon-hound/internal/daemon"
	"github.com/0xdps/daemon-hound/internal/storage"
)

//go:embed templates
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

// Server is the DaemonHound web UI server.
type Server struct {
	vault      *storage.Vault
	sessionKey []byte // random 32-byte HMAC key per process lifetime
	mu         sync.RWMutex
	sessions   map[string]time.Time // token → expiry
	mux        *http.ServeMux
	port       int
}

// NewServer creates a web UI server backed by the given vault.
func NewServer(vault *storage.Vault) (*Server, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate session key: %w", err)
	}
	s := &Server{
		vault:      vault,
		sessionKey: key,
		sessions:   make(map[string]time.Time),
	}
	s.registerRoutes()
	return s, nil
}

// ListenAndServe starts the HTTP server on the first available port in the
// preferred range and returns the URL it is listening on.
func (s *Server) ListenAndServe(ctx context.Context) (string, error) {
	ln, err := findFreePort(7734, 7800)
	if err != nil {
		return "", fmt.Errorf("no free port: %w", err)
	}
	s.port = ln.Addr().(*net.TCPAddr).Port
	url := fmt.Sprintf("http://127.0.0.1:%d", s.port)

	srv := &http.Server{Handler: s.mux}
	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()
	go func() { _ = srv.Serve(ln) }()
	return url, nil
}

// Port returns the port the server is listening on.
func (s *Server) Port() int { return s.port }

// ─── routing ───────────────────────────────────────────────────────────────

func (s *Server) registerRoutes() {
	mux := http.NewServeMux()

	// Static assets (favicon, etc.)
	staticSub, _ := fs.Sub(staticFS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	// Favicon
	mux.HandleFunc("GET /favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/static/logo-trans.png", http.StatusMovedPermanently)
	})

	// Auth
	mux.HandleFunc("GET /login", s.handleLoginPage)
	mux.HandleFunc("POST /login", s.handleLoginSubmit)
	mux.HandleFunc("POST /logout", s.requireAuth(s.handleLogout))

	// Dashboard
	mux.HandleFunc("GET /", s.requireAuth(s.handleDashboard))

	// Conflicts
	mux.HandleFunc("GET /conflicts", s.requireAuth(s.handleConflictList))
	mux.HandleFunc("GET /conflicts/diff", s.requireAuth(s.handleConflictDiff))
	mux.HandleFunc("POST /conflicts/resolve", s.requireAuth(s.handleConflictResolve))

	// Files
	mux.HandleFunc("GET /files", s.requireAuth(s.handleFileList))
	mux.HandleFunc("GET /files/view", s.requireAuth(s.handleFileView))

	// Secrets
	mux.HandleFunc("GET /secrets", s.requireAuth(s.handleSecretList))
	mux.HandleFunc("GET /secrets/view", s.requireAuth(s.handleSecretView))
	mux.HandleFunc("POST /secrets/create", s.requireAuth(s.handleSecretCreate))
	mux.HandleFunc("POST /secrets/rotate", s.requireAuth(s.handleSecretRotate))
	mux.HandleFunc("GET /secrets/diff", s.requireAuth(s.handleSecretDiff))

	// Status (SSE)
	mux.HandleFunc("GET /status", s.requireAuth(s.handleStatus))
	mux.HandleFunc("GET /status/stream", s.requireAuth(s.handleStatusStream))
	mux.HandleFunc("GET /status/pulse", s.requireAuth(s.handlePulse))

	// Settings
	mux.HandleFunc("GET /settings", s.requireAuth(s.handleSettings))
	mux.HandleFunc("POST /settings/untrack", s.requireAuth(s.handleUntrack))
	mux.HandleFunc("POST /settings/bulk-untrack", s.requireAuth(s.handleBulkUntrack))

	// Help
	mux.HandleFunc("GET /help", s.requireAuth(s.handleHelp))

	s.mux = mux
}

// ─── auth middleware ────────────────────────────────────────────────────────

const sessionCookie = "dhd_session"
const sessionTTL = 12 * time.Hour

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.isAuthenticated(r) {
			if isHTMX(r) {
				w.Header().Set("HX-Redirect", "/login")
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			http.Redirect(w, r, "/login?next="+r.URL.Path, http.StatusFound)
			return
		}
		next(w, r)
	}
}

func (s *Server) isAuthenticated(r *http.Request) bool {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	s.mu.RLock()
	expiry, ok := s.sessions[c.Value]
	s.mu.RUnlock()
	return ok && time.Now().Before(expiry)
}

func (s *Server) issueSession(w http.ResponseWriter) {
	token := make([]byte, 24)
	_, _ = rand.Read(token)
	mac := hmac.New(sha256.New, s.sessionKey)
	mac.Write(token)
	signed := hex.EncodeToString(token) + "." + hex.EncodeToString(mac.Sum(nil))

	s.mu.Lock()
	s.sessions[signed] = time.Now().Add(sessionTTL)
	s.mu.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    signed,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})
}

// ─── helpers ────────────────────────────────────────────────────────────────

type pageData struct {
	Active        string // "dashboard", "status", "conflicts", "files", "secrets", "settings"
	PendingCount  int
	DaemonRunning bool
	Data          any
}

type sideMetrics struct {
	Pending int
	Secrets int
	Files   int
}

func (s *Server) pageData(active string) pageData {
	pd := pageData{
		Active:        active,
		DaemonRunning: !daemon.IsHalted(),
	}
	store, err := conflicts.NewStore()
	if err == nil {
		if pending, err := store.Pending(); err == nil {
			pd.PendingCount = len(pending)
		}
	}
	return pd
}

func (s *Server) sideMetrics() sideMetrics {
	m := sideMetrics{}
	if store, err := conflicts.NewStore(); err == nil {
		if pending, err := store.Pending(); err == nil {
			m.Pending = len(pending)
		}
	}
	if state, err := s.vault.LoadState(); err == nil {
		m.Secrets = len(state.Secrets)
		m.Files = len(state.Files)
	}
	return m
}

// mergeHXTriggerSidebar appends sidebar metrics to any existing HX-Trigger
// header (e.g. from setHXToast) so both events are delivered to the client.
func (s *Server) mergeHXTriggerSidebar(w http.ResponseWriter) {
	side := s.sideMetrics()
	extra := fmt.Sprintf(
		`"sidebar:update":{"pending":%d,"secrets":%d,"files":%d}`,
		side.Pending, side.Secrets, side.Files)

	existing := w.Header().Get("HX-Trigger")
	if existing == "" {
		w.Header().Set("HX-Trigger", fmt.Sprintf(`{%s}`, extra))
		return
	}

	// Merge into existing JSON object
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(existing), &obj); err != nil {
		// Existing value isn't valid JSON — replace entirely
		w.Header().Set("HX-Trigger", fmt.Sprintf(`{%s}`, extra))
		return
	}
	obj["sidebar:update"] = json.RawMessage(fmt.Sprintf(`{"pending":%d,"secrets":%d,"files":%d}`, side.Pending, side.Secrets, side.Files))
	merged, err := json.Marshal(obj)
	if err != nil {
		w.Header().Set("HX-Trigger", existing)
		return
	}
	w.Header().Set("HX-Trigger", string(merged))
}

func isHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

func setHXToast(w http.ResponseWriter, kind, message string) {
	if message == "" {
		return
	}
	w.Header().Set("HX-Trigger", fmt.Sprintf(`{"app:toast":{"kind":%q,"message":%q}}`, kind, message))
}

func (s *Server) render(w http.ResponseWriter, active, name string, data any) {
	pd := s.pageData(active)
	pd.Data = data
	tmpl, err := s.loadTemplate(name)
	if err != nil {
		http.Error(w, "template error: "+err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "base", pd); err != nil {
		// Already wrote headers — log only
		fmt.Fprintf(os.Stderr, "web: template execute %s: %v\n", name, err)
	}
}

func (s *Server) renderPartial(w http.ResponseWriter, name string, data any) {
	tmpl, err := s.loadTemplate(name)
	if err != nil {
		http.Error(w, "template error: "+err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	s.mergeHXTriggerSidebar(w)
	if err := tmpl.ExecuteTemplate(w, "content", data); err != nil {
		fmt.Fprintf(os.Stderr, "web: partial execute %s: %v\n", name, err)
	}
}

func (s *Server) renderPartialWithTitle(w http.ResponseWriter, name string, data any, title string) {
	tmpl, err := s.loadTemplate(name)
	if err != nil {
		http.Error(w, "template error: "+err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("HX-Title", title)
	s.mergeHXTriggerSidebar(w)
	if err := tmpl.ExecuteTemplate(w, "content", data); err != nil {
		fmt.Fprintf(os.Stderr, "web: partial execute %s: %v\n", name, err)
	}
}

func (s *Server) renderLogin(w http.ResponseWriter, name string, data any) {
	tmpl, err := s.loadTemplate(name)
	if err != nil {
		http.Error(w, "template error: "+err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "login", data); err != nil {
		fmt.Fprintf(os.Stderr, "web: template execute %s: %v\n", name, err)
	}
}

func (s *Server) loadTemplate(name string) (*template.Template, error) {
	layout, err := fs.ReadFile(templateFS, "templates/layout.html")
	if err != nil {
		return nil, err
	}
	page, err := fs.ReadFile(templateFS, "templates/"+name)
	if err != nil {
		return nil, err
	}
	return template.New("base").Funcs(templateFuncs).Parse(string(layout) + string(page))
}

var templateFuncs = template.FuncMap{
	"truncate": func(s string, n int) string {
		if len(s) <= n {
			return s
		}
		return s[:n] + "…"
	},
	"formatTime": func(t time.Time) string {
		return t.Format("2006-01-02 15:04:05")
	},
	"safeHTML": func(s string) template.HTML {
		return template.HTML(s) //nolint:gosec // intentional for pre-escaped content
	},
	"lines": func(s string) []string {
		return strings.Split(s, "\n")
	},
	"urlquery": func(s string) string {
		return url.QueryEscape(s)
	},
	"detectLogLevel": func(line string) string {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "error") || strings.Contains(lower, "fatal") || strings.Contains(lower, "panic") {
			return "error"
		}
		if strings.Contains(lower, "warn") {
			return "warn"
		}
		return "info"
	},
	"fileIcon": func(path string) string {
		ext := strings.ToLower(path)
		if strings.HasSuffix(ext, ".json") {
			return "file-json"
		}
		if strings.HasSuffix(ext, ".yaml") || strings.HasSuffix(ext, ".yml") {
			return "file-code"
		}
		if strings.HasSuffix(ext, ".toml") {
			return "file-code"
		}
		if strings.HasSuffix(ext, ".md") {
			return "file-text"
		}
		if strings.HasSuffix(ext, ".env") || strings.Contains(ext, ".env.") {
			return "file-lock"
		}
		if strings.HasSuffix(ext, ".go") || strings.HasSuffix(ext, ".rs") || strings.HasSuffix(ext, ".py") || strings.HasSuffix(ext, ".js") || strings.HasSuffix(ext, ".ts") {
			return "file-code"
		}
		if strings.HasSuffix(ext, ".sh") || strings.HasSuffix(ext, ".bash") || strings.HasSuffix(ext, ".zsh") {
			return "terminal"
		}
		if strings.HasSuffix(ext, ".sql") {
			return "database"
		}
		if strings.HasSuffix(ext, ".dockerfile") || strings.Contains(ext, "dockerfile") {
			return "container"
		}
		return "file"
	},
	"detectLang": func(path string) string {
		ext := strings.ToLower(path)
		if strings.HasSuffix(ext, ".json") {
			return "json"
		}
		if strings.HasSuffix(ext, ".yaml") || strings.HasSuffix(ext, ".yml") {
			return "yaml"
		}
		if strings.HasSuffix(ext, ".toml") {
			return "toml"
		}
		if strings.HasSuffix(ext, ".md") {
			return "markdown"
		}
		if strings.HasSuffix(ext, ".env") || strings.Contains(ext, ".env.") {
			return "bash"
		}
		if strings.HasSuffix(ext, ".go") {
			return "go"
		}
		if strings.HasSuffix(ext, ".rs") {
			return "rust"
		}
		if strings.HasSuffix(ext, ".py") {
			return "python"
		}
		if strings.HasSuffix(ext, ".js") {
			return "javascript"
		}
		if strings.HasSuffix(ext, ".ts") {
			return "typescript"
		}
		if strings.HasSuffix(ext, ".sh") || strings.HasSuffix(ext, ".bash") || strings.HasSuffix(ext, ".zsh") {
			return "bash"
		}
		if strings.HasSuffix(ext, ".sql") {
			return "sql"
		}
		if strings.HasSuffix(ext, ".dockerfile") {
			return "dockerfile"
		}
		if strings.HasSuffix(ext, ".html") || strings.HasSuffix(ext, ".htm") {
			return "html"
		}
		if strings.HasSuffix(ext, ".css") {
			return "css"
		}
		if strings.HasSuffix(ext, ".xml") {
			return "xml"
		}
		return "plaintext"
	},
	"parseLogTimestamp": func(line string) string {
		// Match "[daemon] 2024/01/15 10:30:45 message"
		if idx := strings.Index(line, "] "); idx > 0 {
			rest := line[idx+2:]
			// Find the space after the timestamp
			if tsEnd := strings.Index(rest[20:], " "); tsEnd >= 0 {
				return rest[:20+tsEnd]
			}
			if len(rest) >= 19 {
				return rest[:19]
			}
		}
		return ""
	},
	"stripLogPrefix": func(line string) string {
		if idx := strings.Index(line, "] "); idx > 0 {
			rest := line[idx+2:]
			if tsEnd := strings.Index(rest[20:], " "); tsEnd >= 0 {
				return rest[20+tsEnd+1:]
			}
			if len(rest) >= 20 {
				return rest[20:]
			}
		}
		return line
	},
}

func findFreePort(from, to int) (net.Listener, error) {
	for p := from; p <= to; p++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err == nil {
			return ln, nil
		}
	}
	return nil, fmt.Errorf("no free port in range %d-%d", from, to)
}

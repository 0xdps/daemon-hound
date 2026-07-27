package web

import (
	"net/http"

	"github.com/0xdps/daemon-hound/internal/config"
	"github.com/0xdps/daemon-hound/internal/keychain"
	"github.com/0xdps/daemon-hound/internal/utils"
)

type loginPageData struct {
	Error string
	Next  string
}

func (s *Server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	if s.isAuthenticated(r) {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	s.renderLogin(w, "login.html", loginPageData{Next: r.URL.Query().Get("next")})
}

func (s *Server) handleLoginSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", 400)
		return
	}
	password := r.FormValue("password")
	next := r.FormValue("next")
	if next == "" {
		next = "/"
	}

	// Validate password by trying to decrypt the identity file.
	cfg := config.NewConfig()
	if err := cfg.Load(); err != nil {
		s.renderLogin(w, "login.html", loginPageData{Error: "DaemonHound not initialized", Next: next})
		return
	}

	encIdentity, err := readIdentityFile()
	if err != nil {
		s.renderLogin(w, "login.html", loginPageData{Error: "Cannot read identity file", Next: next})
		return
	}

	if _, err := utils.DecryptWithPassword(string(encIdentity), password, cfg.IdentitySalt()); err != nil {
		s.renderLogin(w, "login.html", loginPageData{Error: "Incorrect password", Next: next})
		return
	}

	// Store in keychain for this session so future vault operations work.
	_ = keychain.Store(password)

	s.issueSession(w)
	http.Redirect(w, r, next, http.StatusFound)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		s.mu.Lock()
		delete(s.sessions, c.Value)
		s.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, MaxAge: -1, Path: "/"})
	http.Redirect(w, r, "/login", http.StatusFound)
}

func readIdentityFile() ([]byte, error) {
	return readFile(config.IdentityPath())
}

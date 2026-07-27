package web

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/0xdps/daemon-hound/internal/models"
)

type secretListData struct {
	Secrets []models.SecretIndex
	Names   []string
	Query   string
}

type secretViewData struct {
	Name     string
	SF       *models.SecretFile
	Value    string
	Error    string
	Versions []secretVersion
}

type secretVersion struct {
	Tag       string
	CreatedAt time.Time
	Reason    string
	IsCurrent bool
}

func (s *Server) handleSecretList(w http.ResponseWriter, r *http.Request) {
	state, err := s.vault.LoadState()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	needle := strings.ToLower(query)

	var names []string
	for n := range state.Secrets {
		if needle != "" && !strings.Contains(strings.ToLower(n), needle) {
			continue
		}
		names = append(names, n)
	}
	sort.Strings(names)

	data := secretListData{Names: names, Query: query}
	for _, n := range names {
		data.Secrets = append(data.Secrets, state.Secrets[n])
	}

	if isHTMX(r) {
		s.renderPartialWithTitle(w, "secrets/list.html", data, "Secrets - DaemonHound")
		return
	}
	s.render(w, "secrets", "secrets/list.html", data)
}

func (s *Server) handleSecretView(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		http.Error(w, "name required", 400)
		return
	}

	sf, err := s.vault.LoadSecretFile(name)
	data := secretViewData{Name: name, SF: sf}

	if err != nil {
		data.Error = err.Error()
	} else {
		// Decrypt current value
		ver, ok := sf.Versions[sf.Latest]
		if ok {
			plain, decErr := s.vault.Decrypt(ver.Value)
			if decErr != nil {
				data.Error = decErr.Error()
			} else {
				data.Value = string(plain)
			}
		}

		// Build version list newest-first
		var versions []secretVersion
		for tag, v := range sf.Versions {
			versions = append(versions, secretVersion{
				Tag:       tag,
				CreatedAt: v.CreatedAt,
				Reason:    v.Reason,
				IsCurrent: tag == sf.Latest,
			})
		}
		sort.Slice(versions, func(i, j int) bool {
			return versions[i].CreatedAt.After(versions[j].CreatedAt)
		})
		data.Versions = versions
	}

	if isHTMX(r) {
		s.renderPartialWithTitle(w, "secrets/view.html", data, "Secret View - DaemonHound")
		return
	}
	s.render(w, "secrets", "secrets/view.html", data)
}

func (s *Server) handleSecretRotate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", 400)
		return
	}
	name := r.FormValue("name")
	newValue := r.FormValue("value")
	reason := r.FormValue("reason")
	if name == "" || newValue == "" {
		http.Error(w, "name and value required", 400)
		return
	}
	if reason == "" {
		reason = "Updated via web UI"
	}

	sf, err := s.vault.LoadSecretFile(name)
	if err != nil {
		http.Error(w, fmt.Sprintf("secret not found: %s", name), 404)
		return
	}

	encValue, err := s.vault.Encrypt([]byte(newValue))
	if err != nil {
		http.Error(w, "encrypt failed: "+err.Error(), 500)
		return
	}

	newVer := nextSecretVersion(sf)
	sf.Versions[newVer] = models.SecretVersion{
		CreatedAt: time.Now(),
		Reason:    reason,
		Value:     encValue,
	}
	sf.Latest = newVer

	if err := s.vault.SaveSecretFile(sf); err != nil {
		http.Error(w, "save failed: "+err.Error(), 500)
		return
	}

	if isHTMX(r) {
		setHXToast(w, "success", fmt.Sprintf("Saved new version for %s", name))
		// Return updated view partial
		data := secretViewData{Name: name, SF: sf, Value: newValue}
		var versions []secretVersion
		for tag, v := range sf.Versions {
			versions = append(versions, secretVersion{
				Tag:       tag,
				CreatedAt: v.CreatedAt,
				Reason:    v.Reason,
				IsCurrent: tag == sf.Latest,
			})
		}
		sort.Slice(versions, func(i, j int) bool {
			return versions[i].CreatedAt.After(versions[j].CreatedAt)
		})
		data.Versions = versions
		s.renderPartialWithTitle(w, "secrets/view.html", data, "Secret View - DaemonHound")
		return
	}
	http.Redirect(w, r, "/secrets/view?name="+name, http.StatusFound)
}

// nextSecretVersion increments the version tag (v1 → v2 → v3...).
func nextSecretVersion(sf *models.SecretFile) string {
	max := 0
	for tag := range sf.Versions {
		var n int
		if _, err := fmt.Sscanf(tag, "v%d", &n); err == nil && n > max {
			max = n
		}
	}
	return fmt.Sprintf("v%d", max+1)
}

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
	Value     string // decrypted value for diff comparison in version history
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
			sv := secretVersion{
				Tag:       tag,
				CreatedAt: v.CreatedAt,
				Reason:    v.Reason,
				IsCurrent: tag == sf.Latest,
			}
			// Decrypt value for diff comparison in version history
			if plain, decErr := s.vault.Decrypt(v.Value); decErr == nil {
				sv.Value = string(plain)
			}
			versions = append(versions, sv)
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

func (s *Server) handleSecretCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", 400)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	value := r.FormValue("value")
	reason := r.FormValue("reason")
	if name == "" || value == "" {
		http.Error(w, "name and value required", 400)
		return
	}
	if reason == "" {
		reason = "Created via web UI"
	}

	encValue, err := s.vault.Encrypt([]byte(value))
	if err != nil {
		http.Error(w, "encrypt failed: "+err.Error(), 500)
		return
	}

	sf := &models.SecretFile{
		Name:      name,
		CreatedAt: time.Now(),
		Latest:    "v1",
		Versions: map[string]models.SecretVersion{
			"v1": {
				CreatedAt: time.Now(),
				Reason:    reason,
				Value:     encValue,
			},
		},
	}

	if err := s.vault.SaveSecretFile(sf); err != nil {
		http.Error(w, "save failed: "+err.Error(), 500)
		return
	}

	// Update state index
	state, err := s.vault.LoadState()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if state.Secrets == nil {
		state.Secrets = make(map[string]models.SecretIndex)
	}
	state.Secrets[name] = models.SecretIndex{
		Latest:    "v1",
		Versions:  1,
		UpdatedAt: time.Now(),
	}
	if err := s.vault.SaveState(state); err != nil {
		http.Error(w, "save state failed: "+err.Error(), 500)
		return
	}

	if isHTMX(r) {
		setHXToast(w, "success", fmt.Sprintf("Created secret %s", name))
		// Return updated list partial
		s.handleSecretList(w, r)
		return
	}
	http.Redirect(w, r, "/secrets", http.StatusFound)
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
			sv := secretVersion{
				Tag:       tag,
				CreatedAt: v.CreatedAt,
				Reason:    v.Reason,
				IsCurrent: tag == sf.Latest,
			}
			if plain, decErr := s.vault.Decrypt(v.Value); decErr == nil {
				sv.Value = string(plain)
			}
			versions = append(versions, sv)
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

type secretDiffData struct {
	Name      string
	VersionA  string
	VersionB  string
	ValueA    string
	ValueB    string
	DiffLines []diffLine
}

func (s *Server) handleSecretDiff(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	va := r.URL.Query().Get("a")
	vb := r.URL.Query().Get("b")
	if name == "" || va == "" || vb == "" {
		http.Error(w, "name, a, and b required", 400)
		return
	}

	sf, err := s.vault.LoadSecretFile(name)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	verA, okA := sf.Versions[va]
	verB, okB := sf.Versions[vb]
	if !okA || !okB {
		http.Error(w, "version not found", 404)
		return
	}

	plainA, _ := s.vault.Decrypt(verA.Value)
	plainB, _ := s.vault.Decrypt(verB.Value)

	data := secretDiffData{
		Name:     name,
		VersionA: va,
		VersionB: vb,
		ValueA:   string(plainA),
		ValueB:   string(plainB),
	}

	// Simple line-by-line diff
	linesA := strings.Split(data.ValueA, "\n")
	linesB := strings.Split(data.ValueB, "\n")
	maxLen := len(linesA)
	if len(linesB) > maxLen {
		maxLen = len(linesB)
	}
	for i := 0; i < maxLen; i++ {
		var la, lb string
		if i < len(linesA) {
			la = linesA[i]
		}
		if i < len(linesB) {
			lb = linesB[i]
		}
		var side string
		switch {
		case la == lb:
			side = "same"
		case la == "":
			side = "added"
		case lb == "":
			side = "removed"
		default:
			side = "changed"
		}
		data.DiffLines = append(data.DiffLines, diffLine{
			LineNo:  i + 1,
			Content: la,
			Side:    side,
		})
	}

	if isHTMX(r) {
		s.renderPartialWithTitle(w, "secrets/diff.html", data, fmt.Sprintf("%s diff - DaemonHound", name))
		return
	}
	s.render(w, "secrets", "secrets/diff.html", data)
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

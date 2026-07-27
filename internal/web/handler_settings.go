package web

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/0xdps/daemon-hound/internal/models"
)

type trackedFileEntry struct {
	Key  string // "namespace:relPath"
	File models.TrackedFile
}

type settingsData struct {
	Files []trackedFileEntry
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	state, err := s.vault.LoadState()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	var entries []trackedFileEntry
	for k, f := range state.Files {
		entries = append(entries, trackedFileEntry{Key: k, File: f})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].File.Namespace != entries[j].File.Namespace {
			return entries[i].File.Namespace < entries[j].File.Namespace
		}
		return entries[i].File.RelPath < entries[j].File.RelPath
	})

	data := settingsData{Files: entries}
	if isHTMX(r) {
		s.renderPartialWithTitle(w, "settings.html", data, "Settings - DaemonHound")
		return
	}
	s.render(w, "settings", "settings.html", data)
}

func (s *Server) handleUntrack(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", 400)
		return
	}
	key := r.FormValue("key") // "namespace:relPath"
	if key == "" || !strings.Contains(key, ":") {
		http.Error(w, "key required", 400)
		return
	}

	state, err := s.vault.LoadState()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	file, ok := state.Files[key]
	if !ok {
		http.Error(w, "file not found", 404)
		return
	}

	// Remove from vault (delete encrypted file + state entry)
	if err := s.vault.RemoveFile(file); err != nil {
		http.Error(w, "remove failed: "+err.Error(), 500)
		return
	}
	delete(state.Files, key)
	if err := s.vault.SaveState(state); err != nil {
		http.Error(w, "save state failed: "+err.Error(), 500)
		return
	}

	if isHTMX(r) {
		setHXToast(w, "success", fmt.Sprintf("Untracked %s", file.RelPath))
		// Return the updated settings table
		var entries []trackedFileEntry
		for k, f := range state.Files {
			entries = append(entries, trackedFileEntry{Key: k, File: f})
		}
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Key < entries[j].Key
		})
		s.renderPartialWithTitle(w, "settings.html", settingsData{Files: entries}, "Settings - DaemonHound")
		return
	}
	http.Redirect(w, r, "/settings", http.StatusFound)
}

func (s *Server) handleBulkUntrack(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", 400)
		return
	}
	keysStr := r.FormValue("keys")
	if keysStr == "" {
		http.Error(w, "keys required", 400)
		return
	}

	state, err := s.vault.LoadState()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	keys := strings.Split(keysStr, ",")
	var removed int
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		file, ok := state.Files[key]
		if !ok {
			continue
		}
		if err := s.vault.RemoveFile(file); err != nil {
			continue
		}
		delete(state.Files, key)
		removed++
	}

	if err := s.vault.SaveState(state); err != nil {
		http.Error(w, "save state failed: "+err.Error(), 500)
		return
	}

	if isHTMX(r) {
		setHXToast(w, "success", fmt.Sprintf("Untracked %d files", removed))
		var entries []trackedFileEntry
		for k, f := range state.Files {
			entries = append(entries, trackedFileEntry{Key: k, File: f})
		}
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Key < entries[j].Key
		})
		s.renderPartialWithTitle(w, "settings.html", settingsData{Files: entries}, "Settings - DaemonHound")
		return
	}
	http.Redirect(w, r, "/settings", http.StatusFound)
}

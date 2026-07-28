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
	"strings"
	"time"

	"github.com/0xdps/daemon-hound/internal/config"
	"github.com/0xdps/daemon-hound/internal/conflicts"
	"github.com/0xdps/daemon-hound/internal/daemon"
	"github.com/0xdps/daemon-hound/internal/git"
	"github.com/0xdps/daemon-hound/internal/merge"
)

type conflictListData struct {
	Conflicts []*conflicts.Conflict
	Pending   []*conflicts.Conflict
	Resolved  []*conflicts.Conflict
	Halted    bool
}

type diffLine struct {
	LineNo  int
	Content string
	Side    string // "local", "remote", "both", "base"
}

type conflictDiffData struct {
	FilePath    string
	LocalLines  []string
	RemoteLines []string
	BaseLines   []string
	// For env/toml files: key-level diff
	KeyDiffs []keyDiff
	IsKV     bool // true when we can show key-value level diff
}

type keyDiff struct {
	Key         string
	LocalValue  string
	RemoteValue string
	BaseValue   string
	Status      string // "conflict", "local-only", "remote-only", "same"
}

func (s *Server) handleConflictList(w http.ResponseWriter, r *http.Request) {
	store, err := conflicts.NewStore()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	all, err := store.List()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	data := conflictListData{
		Conflicts: all,
		Halted:    daemon.IsHalted(),
	}
	for _, conflict := range all {
		if conflict.ResolvedAt == nil {
			data.Pending = append(data.Pending, conflict)
			continue
		}
		data.Resolved = append(data.Resolved, conflict)
	}
	if isHTMX(r) {
		s.renderPartialWithTitle(w, "conflicts/list.html", data, "Conflicts - DaemonHound")
		return
	}
	s.render(w, "conflicts", "conflicts/list.html", data)
}

func (s *Server) handleConflictDiff(w http.ResponseWriter, r *http.Request) {
	filePath := r.URL.Query().Get("file")
	if filePath == "" {
		http.Error(w, "file parameter required", 400)
		return
	}

	gc := git.NewClient(config.VaultPath())
	baseEnc, localEnc, remoteEnc, err := gc.GetConflictVersions(filePath)
	if err != nil {
		// Conflict may have been committed already — load from store
		store, _ := conflicts.NewStore()
		c, _ := store.Get(filePath)
		if c == nil {
			http.Error(w, "conflict not found and no git versions available", 404)
			return
		}
		// Show hashes only
		data := conflictDiffData{FilePath: filePath}
		s.renderConflictDiff(w, r, data)
		return
	}

	isEncrypted := strings.HasSuffix(filePath, ".age")

	var base, local, remote []byte
	if isEncrypted {
		base, _ = s.vault.Decrypt(baseEnc)
		local, _ = s.vault.Decrypt(localEnc)
		remote, _ = s.vault.Decrypt(remoteEnc)
	} else {
		base, local, remote = baseEnc, localEnc, remoteEnc
	}

	data := conflictDiffData{
		FilePath:    filePath,
		BaseLines:   splitLines(string(base)),
		LocalLines:  splitLines(string(local)),
		RemoteLines: splitLines(string(remote)),
	}

	// If it looks like a key=value file, build key-level diff
	if isKVFile(filePath) {
		data.IsKV = true
		data.KeyDiffs = buildKeyDiffs(base, local, remote)
	}

	s.renderConflictDiff(w, r, data)
}

func (s *Server) renderConflictDiff(w http.ResponseWriter, r *http.Request, data conflictDiffData) {
	if isHTMX(r) {
		s.renderPartial(w, "conflicts/diff.html", data)
		return
	}
	s.render(w, "conflicts", "conflicts/diff.html", data)
}

func (s *Server) handleConflictResolve(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", 400)
		return
	}
	filePath := r.FormValue("file")
	strategy := r.FormValue("strategy")

	if filePath == "" || (strategy != "local" && strategy != "remote" && strategy != "merged") {
		http.Error(w, "file and valid strategy required", 400)
		return
	}

	store, err := conflicts.NewStore()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	gc := git.NewClient(config.VaultPath())

	if strategy == "merged" {
		mergedContent := r.FormValue("merged_content")
		if mergedContent != "" {
			// User submitted a custom inline merge
			reenc, err := s.vault.Encrypt([]byte(mergedContent))
			if err == nil {
				_ = gc.StageFile(filePath, reenc)
			}
		} else {
			// Re-run smart merge
			baseEnc, localEnc, remoteEnc, err := gc.GetConflictVersions(filePath)
			if err == nil {
				base, _ := s.vault.Decrypt(baseEnc)
				local, _ := s.vault.Decrypt(localEnc)
				remote, _ := s.vault.Decrypt(remoteEnc)

				reg := merge.NewRegistry().WithSecretDriver(s.vault.Encrypt, s.vault.Decrypt)
				merged, result, _ := reg.Resolve(filePath, base, local, remote)
				if result == merge.Merged {
					reenc, err := s.vault.Encrypt(merged)
					if err == nil {
						_ = gc.StageFile(filePath, reenc)
						strategy = "merged"
					}
				}
			}
		}
	} else if gc.IsInMerge() {
		switch strategy {
		case "local":
			_ = gc.CheckoutOurs(filePath)
		case "remote":
			_ = gc.CheckoutTheirs(filePath)
		}
		_ = gc.StageOnly(filePath)
	}

	if gc.IsInMerge() {
		_ = gc.CommitAll(fmt.Sprintf("daemon-hound: resolve conflict in %s [%s via web]", filePath, strategy))
	}

	// Mark resolved
	now := time.Now()
	c := &conflicts.Conflict{FilePath: filePath}
	if existing, err := store.Get(filePath); err == nil {
		c = existing
	}
	c.ResolvedAt = &now
	c.ResolutionStrategy = strategy
	_ = store.Add(c)

	// Check if all conflicts are resolved — if so, clear halt
	if pending, err := store.Pending(); err == nil && len(pending) == 0 {
		_ = daemon.ClearHalt()
	}

	if isHTMX(r) {
		setHXToast(w, "success", fmt.Sprintf("Resolved %s using %s", filePath, strategy))
		// Return updated conflict list partial
		all, _ := store.List()
		data := conflictListData{Conflicts: all, Halted: daemon.IsHalted()}
		for _, conflict := range all {
			if conflict.ResolvedAt == nil {
				data.Pending = append(data.Pending, conflict)
				continue
			}
			data.Resolved = append(data.Resolved, conflict)
		}
		s.renderPartial(w, "conflicts/list.html", data)
		return
	}
	http.Redirect(w, r, "/conflicts", http.StatusFound)
}

// ─── helpers ────────────────────────────────────────────────────────────────

func splitLines(s string) []string {
	return strings.Split(strings.TrimRight(s, "\n"), "\n")
}

func isKVFile(filePath string) bool {
	lower := strings.ToLower(filePath)
	return strings.HasSuffix(lower, ".env") ||
		strings.Contains(lower, ".env.") ||
		strings.HasSuffix(lower, ".env.age") ||
		strings.Contains(lower, ".env.") ||
		strings.HasSuffix(lower, "state.toml") ||
		strings.HasSuffix(lower, "state.toml.age")
}

func buildKeyDiffs(base, local, remote []byte) []keyDiff {
	bm, _ := parseKV(base)
	lm, _ := parseKV(local)
	rm, _ := parseKV(remote)

	seen := map[string]bool{}
	var order []string
	for k := range bm {
		if !seen[k] {
			order = append(order, k)
			seen[k] = true
		}
	}
	for k := range lm {
		if !seen[k] {
			order = append(order, k)
			seen[k] = true
		}
	}
	for k := range rm {
		if !seen[k] {
			order = append(order, k)
			seen[k] = true
		}
	}

	var diffs []keyDiff
	for _, k := range order {
		bv := bm[k]
		lv := lm[k]
		rv := rm[k]

		_, inL := lm[k]
		_, inR := rm[k]

		var status string
		switch {
		case !inL && !inR:
			continue // deleted on both
		case !inL:
			status = "remote-only"
		case !inR:
			status = "local-only"
		case lv == rv:
			status = "same"
		case bv == lv:
			status = "remote-only" // only remote changed
		case bv == rv:
			status = "local-only" // only local changed
		default:
			status = "conflict"
		}
		diffs = append(diffs, keyDiff{
			Key:         k,
			LocalValue:  lv,
			RemoteValue: rv,
			BaseValue:   bv,
			Status:      status,
		})
	}
	return diffs
}

func parseKV(data []byte) (map[string]string, []string) {
	m := make(map[string]string)
	var order []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			continue
		}
		k := strings.TrimSpace(line[:idx])
		v := strings.TrimSpace(line[idx+1:])
		if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[0] == v[len(v)-1] {
			v = v[1 : len(v)-1]
		}
		if k != "" {
			m[k] = v
			order = append(order, k)
		}
	}
	return m, order
}

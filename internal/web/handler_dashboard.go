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
	"time"

	"github.com/0xdps/daemon-hound/internal/conflicts"
	"github.com/0xdps/daemon-hound/internal/daemon"
)

type dashboardData struct {
	Halted       bool
	HaltReason   string
	PendingCount int
	SecretCount  int
	FileCount    int
	LastActivity string
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	data := dashboardData{
		Halted:     daemon.IsHalted(),
		HaltReason: daemon.ReadHaltReason(),
	}

	if store, err := conflicts.NewStore(); err == nil {
		if pending, err := store.Pending(); err == nil {
			data.PendingCount = len(pending)
		}
	}

	if state, err := s.vault.LoadState(); err == nil {
		data.SecretCount = len(state.Secrets)
		data.FileCount = len(state.Files)
		// Find most recent sync activity
		var latest time.Time
		for _, f := range state.Files {
			if f.LastSyncAt.After(latest) {
				latest = f.LastSyncAt
			}
		}
		if !latest.IsZero() {
			data.LastActivity = formatRelativeTime(latest)
		}
	}

	title := "Dashboard - DaemonHound"
	if data.PendingCount > 0 {
		title = fmt.Sprintf("(%d) %s", data.PendingCount, title)
	}

	if isHTMX(r) {
		s.renderPartialWithTitle(w, "dashboard.html", data, title)
		return
	}
	s.render(w, "dashboard", "dashboard.html", data)
}

func formatRelativeTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%d min ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d hr ago", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%d days ago", int(d.Hours()/24))
	default:
		return t.Format("2006-01-02")
	}
}

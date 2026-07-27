package web

import (
	"fmt"
	"net/http"

	"github.com/0xdps/daemon-hound/internal/conflicts"
	"github.com/0xdps/daemon-hound/internal/daemon"
)

type dashboardData struct {
	Halted       bool
	HaltReason   string
	PendingCount int
	SecretCount  int
	FileCount    int
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

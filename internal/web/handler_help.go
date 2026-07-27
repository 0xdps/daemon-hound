package web

import (
	"net/http"
	"os"
	"runtime"

	"github.com/0xdps/daemon-hound/internal/daemon"
)

type helpData struct {
	Version   string
	DaemonRun bool
	GoVersion string
	OS        string
	Arch      string
	Port      int
}

func (s *Server) handleHelp(w http.ResponseWriter, r *http.Request) {
	data := helpData{
		Version:   getVersion(),
		DaemonRun: !daemon.IsHalted(),
		GoVersion: runtime.Version(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		Port:      s.port,
	}

	if isHTMX(r) {
		s.renderPartialWithTitle(w, "help.html", data, "Help - DaemonHound")
		return
	}
	s.render(w, "help", "help.html", data)
}

func getVersion() string {
	if v := os.Getenv("DHD_VERSION"); v != "" {
		return v
	}
	return "dev"
}

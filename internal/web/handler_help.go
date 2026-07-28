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

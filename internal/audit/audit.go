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

package audit

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func logPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".daemon-hound", "audit.log")
}

// Log appends a timestamped entry to ~/.daemon-hound/audit.log.
// Errors are silently ignored — logging must never break a command.
func Log(action, detail string) {
	f, err := os.OpenFile(logPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s  %-16s  %s\n", time.Now().UTC().Format(time.RFC3339), action, detail)
}

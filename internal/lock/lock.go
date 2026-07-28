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

package lock

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// Lock manages an exclusive process lock backed by a PID file.
// Stale locks (from crashed processes) are cleaned up automatically.
type Lock struct {
	path string
	held bool
}

// New returns a Lock scoped to the given directory.
func New(dir string) *Lock {
	return &Lock{path: filepath.Join(dir, "sync.lock")}
}

// Acquire attempts to obtain the lock.
// Returns an error if another live process holds it.
func (l *Lock) Acquire() error {
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err == nil {
		fmt.Fprintf(f, "%d", os.Getpid())
		f.Close()
		l.held = true
		return nil
	}
	if !os.IsExist(err) {
		return fmt.Errorf("failed to create lock file: %w", err)
	}

	// Lock file exists — check if the holding PID is still alive.
	data, readErr := os.ReadFile(l.path)
	if readErr != nil {
		// Can't read it; assume stale and steal.
		_ = os.Remove(l.path)
		return l.Acquire()
	}

	pid, parseErr := strconv.Atoi(strings.TrimSpace(string(data)))
	if parseErr != nil || !processAlive(pid) {
		// Stale lock.
		_ = os.Remove(l.path)
		return l.Acquire()
	}

	return fmt.Errorf("another dhd process (PID %d) is running — if it has exited, delete %s", pid, l.path)
}

// Release releases the lock and removes the lock file.
func (l *Lock) Release() {
	if l.held {
		_ = os.Remove(l.path)
		l.held = false
	}
}

// processAlive reports whether the given PID is running on this machine.
func processAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, FindProcess always succeeds; send signal 0 to probe liveness.
	return p.Signal(syscall.Signal(0)) == nil
}

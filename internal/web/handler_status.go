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
	"bufio"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/0xdps/daemon-hound/internal/daemon"
)

type statusData struct {
	Halted     bool
	HaltReason string
	LogPath    string
	RecentLogs []string
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	logPath := daemon.GetLogPath()
	data := statusData{
		Halted:     daemon.IsHalted(),
		HaltReason: daemon.ReadHaltReason(),
		LogPath:    logPath,
	}

	data.RecentLogs = tailFile(logPath, 50)

	if isHTMX(r) {
		s.renderPartialWithTitle(w, "status.html", data, "Live Status - DaemonHound")
		return
	}
	s.render(w, "status", "status.html", data)
}

// handlePulse returns a lightweight JSON status for the sidebar polling.
func (s *Server) handlePulse(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"running":%t}`, !daemon.IsHalted())
}

// handleStatusStream streams daemon log lines as Server-Sent Events so the
// browser page updates in real-time without polling.
func (s *Server) handleStatusStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", 500)
		return
	}

	logPath := daemon.GetLogPath()
	f, err := os.Open(logPath)
	if err != nil {
		fmt.Fprintf(w, "event: error\ndata: cannot open log file\n\n")
		flusher.Flush()
		return
	}
	defer f.Close()

	// Seek to end first — only stream new lines
	_, _ = f.Seek(0, 2)
	scanner := bufio.NewScanner(f)

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			for scanner.Scan() {
				line := scanner.Text()
				level := detectLogLevel(line)
				ts := parseLogTimestamp(line)
				msg := stripLogPrefix(line)
				fmt.Fprintf(w, "event: log\ndata: <div class=\"log-line\" data-level=\"%s\"><span class=\"log-ts\">%s</span><span class=\"log-msg\">%s</span></div>\n\n", level, ts, msg)
				flusher.Flush()
			}
			_ = scanner.Err() // non-fatal; retry on next tick
			// Send heartbeat to keep connection alive
			fmt.Fprintf(w, ": heartbeat\n\n")
			flusher.Flush()
		}
	}
}

func detectLogLevel(line string) string {
	lower := strings.ToLower(line)
	if strings.Contains(lower, "error") || strings.Contains(lower, "fatal") || strings.Contains(lower, "panic") {
		return "error"
	}
	if strings.Contains(lower, "warn") {
		return "warn"
	}
	return "info"
}

func parseLogTimestamp(line string) string {
	if idx := strings.Index(line, "] "); idx > 0 {
		rest := line[idx+2:]
		if tsEnd := strings.Index(rest[20:], " "); tsEnd >= 0 {
			return rest[:20+tsEnd]
		}
		if len(rest) >= 19 {
			return rest[:19]
		}
	}
	return ""
}

func stripLogPrefix(line string) string {
	if idx := strings.Index(line, "] "); idx > 0 {
		rest := line[idx+2:]
		if tsEnd := strings.Index(rest[20:], " "); tsEnd >= 0 {
			return rest[20+tsEnd+1:]
		}
		if len(rest) >= 20 {
			return rest[20:]
		}
	}
	return line
}

func tailFile(path string, n int) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return []string{"(log file not found)"}
	}
	scanner := bufio.NewScanner(
		// Scan lines from end
		&byteReader{data: data},
	)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	_ = scanner.Err()
	if len(lines) <= n {
		return lines
	}
	return lines[len(lines)-n:]
}

type byteReader struct {
	data []byte
	pos  int
}

func (b *byteReader) Read(p []byte) (int, error) {
	if b.pos >= len(b.data) {
		return 0, fmt.Errorf("EOF")
	}
	n := copy(p, b.data[b.pos:])
	b.pos += n
	return n, nil
}

package web

import (
	"bufio"
	"fmt"
	"net/http"
	"os"
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
				fmt.Fprintf(w, "event: log\ndata: %s\n\n", line)
				flusher.Flush()
			}
			_ = scanner.Err() // non-fatal; retry on next tick
			// Send heartbeat to keep connection alive
			fmt.Fprintf(w, ": heartbeat\n\n")
			flusher.Flush()
		}
	}
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

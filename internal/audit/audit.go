package audit

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func logPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".dh", "audit.log")
}

// Log appends a timestamped entry to ~/.dh/audit.log.
// Errors are silently ignored — logging must never break a command.
func Log(action, detail string) {
	f, err := os.OpenFile(logPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s  %-16s  %s\n", time.Now().UTC().Format(time.RFC3339), action, detail)
}

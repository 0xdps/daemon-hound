package merge

import "strings"

// Result indicates the outcome of a merge attempt.
type Result int

const (
	Merged      Result = iota // Merged cleanly, no user action needed
	HasConflict               // True conflict — needs user to decide
	Unsupported               // No driver can handle this file type
)

// Driver performs 3-way content merge for a specific file type.
type Driver interface {
	// CanHandle reports whether this driver handles the given filename.
	CanHandle(filename string) bool
	// Merge performs a 3-way merge: base is the common ancestor, local and remote
	// are the two diverged versions. Returns merged content and result.
	Merge(base, local, remote []byte) (merged []byte, result Result, err error)
}

// Registry holds merge drivers in priority order.
type Registry struct {
	drivers []Driver
}

// NewRegistry returns a registry pre-loaded with all built-in drivers.
// Drivers are tried in order; the first one whose CanHandle returns true is used.
func NewRegistry() *Registry {
	return &Registry{
		drivers: []Driver{
			&StateDriver{}, // state.toml.age — highest priority, special handling
			&EnvDriver{},   // *.env, .env.*, .envrc
			&JSONDriver{},  // *.json
			&CSVDriver{},   // *.csv
			&TextDriver{},  // generic text fallback
		},
	}
}

// Resolve finds the appropriate driver for filename and attempts a 3-way merge.
//   - Returns (merged, Merged, nil) on clean merge.
//   - Returns (nil, HasConflict, nil) when a true conflict requires user input.
//   - Returns (nil, Unsupported, nil) when no driver can handle the file.
func (r *Registry) Resolve(filename string, base, local, remote []byte) ([]byte, Result, error) {
	// Strip trailing path component to get bare filename for matching
	bare := filename
	if idx := strings.LastIndexByte(filename, '/'); idx >= 0 {
		bare = filename[idx+1:]
	}

	for _, d := range r.drivers {
		if d.CanHandle(bare) {
			return d.Merge(base, local, remote)
		}
	}
	return nil, Unsupported, nil
}

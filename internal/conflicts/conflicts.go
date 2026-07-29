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

package conflicts

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Conflict represents a merge conflict detected by the daemon.
type Conflict struct {
	FilePath           string     `json:"file_path"`                     // Relative path in vault (e.g., "vault/secrets/prod.env.age")
	LocalHash          string     `json:"local_hash"`                    // Hash of local version
	RemoteHash         string     `json:"remote_hash"`                   // Hash of remote version
	DetectedAt         time.Time  `json:"detected_at"`                   // When conflict was first detected
	ResolvedAt         *time.Time `json:"resolved_at,omitempty"`         // When conflict was resolved (null if pending)
	ResolutionStrategy string     `json:"resolution_strategy,omitempty"` // "local", "remote", "manual", etc.
	LocalContent       string     `json:"local_content,omitempty"`       // Decrypted local content (if available)
	RemoteContent      string     `json:"remote_content,omitempty"`      // Decrypted remote content (if available)
}

// Store manages conflict storage and retrieval.
type Store struct {
	storePath string
}

// NewStore creates a conflict store at ~/.daemon-hound/conflicts.json
func NewStore() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	storePath := filepath.Join(home, ".daemon-hound", "conflicts.json")
	return &Store{storePath: storePath}, nil
}

// Add adds or updates a conflict.
func (s *Store) Add(conflict *Conflict) error {
	conflicts, err := s.List()
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	// Check if conflict already exists and update it
	found := false
	for i, c := range conflicts {
		if c.FilePath == conflict.FilePath {
			conflicts[i] = conflict
			found = true
			break
		}
	}

	if !found {
		conflicts = append(conflicts, conflict)
	}

	return s.save(conflicts)
}

// List returns all conflicts (both pending and resolved).
func (s *Store) List() ([]*Conflict, error) {
	data, err := os.ReadFile(s.storePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []*Conflict{}, nil
		}
		return nil, fmt.Errorf("failed to read conflicts store: %w", err)
	}

	var conflicts []*Conflict
	if err := json.Unmarshal(data, &conflicts); err != nil {
		return nil, fmt.Errorf("failed to parse conflicts store: %w", err)
	}

	return conflicts, nil
}

// Pending returns only unresolved conflicts.
func (s *Store) Pending() ([]*Conflict, error) {
	all, err := s.List()
	if err != nil {
		return nil, err
	}

	var pending []*Conflict
	for _, c := range all {
		if c.ResolvedAt == nil {
			pending = append(pending, c)
		}
	}

	return pending, nil
}

// Get retrieves a specific conflict by file path.
func (s *Store) Get(filePath string) (*Conflict, error) {
	conflicts, err := s.List()
	if err != nil {
		return nil, err
	}

	for _, c := range conflicts {
		if c.FilePath == filePath {
			return c, nil
		}
	}

	return nil, fmt.Errorf("conflict not found: %s", filePath)
}

// Resolve marks a conflict as resolved with the given strategy.
func (s *Store) Resolve(filePath, strategy string) error {
	conflict, err := s.Get(filePath)
	if err != nil {
		return err
	}

	now := time.Now()
	conflict.ResolvedAt = &now
	conflict.ResolutionStrategy = strategy

	return s.Add(conflict)
}

// Delete removes a conflict record.
func (s *Store) Delete(filePath string) error {
	conflicts, err := s.List()
	if err != nil {
		return err
	}

	var filtered []*Conflict
	for _, c := range conflicts {
		if c.FilePath != filePath {
			filtered = append(filtered, c)
		}
	}

	return s.save(filtered)
}

// save writes conflicts to disk.
func (s *Store) save(conflicts []*Conflict) error {
	data, err := json.MarshalIndent(conflicts, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal conflicts: %w", err)
	}

	if err := os.WriteFile(s.storePath, data, 0600); err != nil {
		return fmt.Errorf("failed to write conflicts store: %w", err)
	}

	return nil
}

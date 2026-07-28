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

package merge

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
)

// EnvDriver performs 3-way merge of .env style key=value files.
// Rules:
//   - Machine A adds KEY_A, Machine B adds KEY_B → merge both
//   - Machine A changes DB_PASS, Machine B doesn't touch it → take A's value
//   - Both machines change DB_PASS to different values → true conflict
//   - Both change DB_PASS to the SAME value → merge (deduplication)
type EnvDriver struct{}

func (d *EnvDriver) CanHandle(filename string) bool {
	// Matches: .env, .env.local, .env.production, production.env, .envrc
	return filename == ".env" ||
		filename == ".envrc" ||
		strings.HasPrefix(filename, ".env.") ||
		strings.HasSuffix(filename, ".env")
}

func (d *EnvDriver) Merge(base, local, remote []byte) ([]byte, Result, error) {
	baseMap, baseOrder := parseEnv(base)
	localMap, _ := parseEnv(local)
	remoteMap, _ := parseEnv(remote)

	// Collect all keys in stable order: base order first, then new keys from local, then remote.
	seen := make(map[string]bool)
	var orderedKeys []string
	for _, k := range baseOrder {
		orderedKeys = append(orderedKeys, k)
		seen[k] = true
	}
	for k := range localMap {
		if !seen[k] {
			orderedKeys = append(orderedKeys, k)
			seen[k] = true
		}
	}
	for k := range remoteMap {
		if !seen[k] {
			orderedKeys = append(orderedKeys, k)
			seen[k] = true
		}
	}

	merged := make(map[string]string)
	hasConflict := false

	for _, key := range orderedKeys {
		bv, inBase := baseMap[key]
		lv, inLocal := localMap[key]
		rv, inRemote := remoteMap[key]

		localChanged := inLocal && (!inBase || bv != lv)
		remoteChanged := inRemote && (!inBase || bv != rv)

		switch {
		case !inLocal && !inRemote:
			// Deleted by both — omit
		case inBase && !inLocal && remoteChanged:
			hasConflict = true // local deleted, remote changed
		case inBase && !inRemote && localChanged:
			hasConflict = true // remote deleted, local changed
		case !inLocal:
			// local-only delete — omit
		case !inRemote:
			// remote-only delete — omit
		case localChanged && remoteChanged:
			if lv == rv {
				merged[key] = lv // same change, take once
			} else {
				hasConflict = true // true conflict
			}
		case localChanged:
			merged[key] = lv
		case remoteChanged:
			merged[key] = rv
		default:
			merged[key] = lv // unchanged
		}
	}

	if hasConflict {
		return nil, HasConflict, nil
	}

	// Serialise in stable order (skip deleted keys)
	var buf bytes.Buffer
	for _, k := range orderedKeys {
		if v, ok := merged[k]; ok {
			fmt.Fprintf(&buf, "%s=%s\n", k, v)
		}
	}

	return buf.Bytes(), Merged, nil
}

// parseEnv parses KEY=VALUE lines. Returns a map and the ordered list of keys.
// Handles comments (#), blank lines, and quoted values.
func parseEnv(data []byte) (map[string]string, []string) {
	m := make(map[string]string)
	var order []string

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		// Strip optional surrounding quotes
		if len(val) >= 2 && (val[0] == '"' || val[0] == '\'') && val[0] == val[len(val)-1] {
			val = val[1 : len(val)-1]
		}
		if key != "" {
			m[key] = val
			order = append(order, key)
		}
	}
	return m, order
}

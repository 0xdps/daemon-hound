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
	"encoding/json"
	"fmt"
	"strings"
)

// JSONDriver performs deep 3-way merge of JSON files.
// Two machines adding different top-level keys → auto-merges.
// Both machines changing the same leaf value to different values → true conflict.
type JSONDriver struct{}

func (d *JSONDriver) CanHandle(filename string) bool {
	return strings.HasSuffix(filename, ".json")
}

func (d *JSONDriver) Merge(base, local, remote []byte) ([]byte, Result, error) {
	var baseV, localV, remoteV interface{}

	if err := json.Unmarshal(base, &baseV); err != nil {
		return nil, HasConflict, fmt.Errorf("parse base JSON: %w", err)
	}
	if err := json.Unmarshal(local, &localV); err != nil {
		return nil, HasConflict, fmt.Errorf("parse local JSON: %w", err)
	}
	if err := json.Unmarshal(remote, &remoteV); err != nil {
		return nil, HasConflict, fmt.Errorf("parse remote JSON: %w", err)
	}

	merged, ok := mergeJSON(baseV, localV, remoteV)
	if !ok {
		return nil, HasConflict, nil
	}

	out, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return nil, HasConflict, fmt.Errorf("encode merged JSON: %w", err)
	}
	return out, Merged, nil
}

// mergeJSON recursively merges JSON values. Returns (merged, true) or (nil, false) on conflict.
func mergeJSON(base, local, remote interface{}) (interface{}, bool) {
	// If all three are maps, do field-level merge
	baseMap, baseIsMap := base.(map[string]interface{})
	localMap, localIsMap := local.(map[string]interface{})
	remoteMap, remoteIsMap := remote.(map[string]interface{})

	if baseIsMap && localIsMap && remoteIsMap {
		out := make(map[string]interface{})
		allKeys := make(map[string]struct{})
		for k := range baseMap {
			allKeys[k] = struct{}{}
		}
		for k := range localMap {
			allKeys[k] = struct{}{}
		}
		for k := range remoteMap {
			allKeys[k] = struct{}{}
		}

		for key := range allKeys {
			bv, inBase := baseMap[key]
			lv, inLocal := localMap[key]
			rv, inRemote := remoteMap[key]

			switch {
			case !inLocal && !inRemote:
				// Deleted by both
			case inBase && !inLocal && inRemote:
				// Local deleted it; only keep if remote also didn't change it
				if jsonEqual(bv, rv) {
					// remote unchanged, local deleted — apply delete
				} else {
					return nil, false // conflict
				}
			case inBase && inLocal && !inRemote:
				// Remote deleted it
				if jsonEqual(bv, lv) {
					// local unchanged, remote deleted — apply delete
				} else {
					return nil, false // conflict
				}
			case !inBase && inLocal && inRemote:
				// Both added this key
				merged, ok := mergeJSON(nil, lv, rv)
				if !ok {
					return nil, false
				}
				out[key] = merged
			default:
				// Present in at least one side with a base
				var baseVal interface{}
				if inBase {
					baseVal = bv
				}
				merged, ok := mergeJSON(baseVal, lv, rv)
				if !ok {
					return nil, false
				}
				out[key] = merged
			}
		}
		return out, true
	}

	// For arrays and scalar values, do a simple 3-way pick
	localSame := jsonEqual(base, local)
	remoteSame := jsonEqual(base, remote)

	switch {
	case localSame && remoteSame:
		return local, true
	case localSame:
		return remote, true // only remote changed
	case remoteSame:
		return local, true // only local changed
	default:
		if jsonEqual(local, remote) {
			return local, true // both changed to same value
		}
		return nil, false // true conflict
	}
}

// jsonEqual compares two JSON values for equality via re-marshalling.
func jsonEqual(a, b interface{}) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	aj, _ := json.Marshal(a)
	bj, _ := json.Marshal(b)
	return string(aj) == string(bj)
}

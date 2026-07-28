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
	"bytes"
	"encoding/csv"
	"fmt"
	"strings"
)

// CSVDriver performs 3-way merge of CSV files.
// Strategy: rows are identified by their first-column value (primary key).
// Machine A adds row for "user_id=5", Machine B adds row for "user_id=6" → both merged.
// Machine A changes a cell in row "user_id=5", Machine B also changes same cell differently → conflict.
// New columns added by one side → added to all rows (with empty value for existing rows).
type CSVDriver struct{}

func (d *CSVDriver) CanHandle(filename string) bool {
	return strings.HasSuffix(filename, ".csv")
}

func (d *CSVDriver) Merge(base, local, remote []byte) ([]byte, Result, error) {
	baseRows, baseHeaders, err := parseCSV(base)
	if err != nil {
		return nil, HasConflict, fmt.Errorf("parse base CSV: %w", err)
	}
	localRows, localHeaders, err := parseCSV(local)
	if err != nil {
		return nil, HasConflict, fmt.Errorf("parse local CSV: %w", err)
	}
	remoteRows, remoteHeaders, err := parseCSV(remote)
	if err != nil {
		return nil, HasConflict, fmt.Errorf("parse remote CSV: %w", err)
	}

	if len(baseHeaders) == 0 {
		// Empty base — just concatenate (dedup by first column if possible)
		return concatCSV(localRows, remoteRows, localHeaders, remoteHeaders)
	}

	// Merge headers: union of all columns in stable order
	mergedHeaders := mergeHeaders(baseHeaders, localHeaders, remoteHeaders)

	// Index rows by first-column value
	baseIdx := indexByFirstCol(baseRows)
	localIdx := indexByFirstCol(localRows)
	remoteIdx := indexByFirstCol(remoteRows)

	// Stable row order: base rows first, then new rows from local, then new from remote
	seenKeys := make(map[string]bool)
	var orderedKeys []string
	for _, row := range baseRows {
		if len(row) > 0 {
			orderedKeys = append(orderedKeys, row[0])
			seenKeys[row[0]] = true
		}
	}
	for _, row := range localRows {
		if len(row) > 0 && !seenKeys[row[0]] {
			orderedKeys = append(orderedKeys, row[0])
			seenKeys[row[0]] = true
		}
	}
	for _, row := range remoteRows {
		if len(row) > 0 && !seenKeys[row[0]] {
			orderedKeys = append(orderedKeys, row[0])
			seenKeys[row[0]] = true
		}
	}

	var mergedRows [][]string
	hasConflict := false

	for _, key := range orderedKeys {
		br := toColMap(baseIdx[key], baseHeaders)
		lr := toColMap(localIdx[key], localHeaders)
		rr := toColMap(remoteIdx[key], remoteHeaders)

		inBase := baseIdx[key] != nil
		inLocal := localIdx[key] != nil
		inRemote := remoteIdx[key] != nil

		if !inLocal && !inRemote {
			continue // deleted by both
		}
		if inBase && !inLocal && inRemote {
			// local deleted, remote has it
			if rowsEqual(br, rr) {
				continue // remote unchanged, local deleted — apply delete
			}
			hasConflict = true // remote changed it, local deleted — conflict
			continue
		}
		if inBase && inLocal && !inRemote {
			if rowsEqual(br, lr) {
				continue // local unchanged, remote deleted — apply delete
			}
			hasConflict = true
			continue
		}

		// Merge cell-by-cell across all headers
		mergedRow := make(map[string]string)
		for _, col := range mergedHeaders {
			bv := br[col]
			lv := lr[col]
			rv := rr[col]

			localColChanged := inLocal && lv != bv
			remoteColChanged := inRemote && rv != bv

			switch {
			case localColChanged && remoteColChanged:
				if lv == rv {
					mergedRow[col] = lv
				} else {
					hasConflict = true
				}
			case localColChanged:
				mergedRow[col] = lv
			case remoteColChanged:
				mergedRow[col] = rv
			default:
				mergedRow[col] = bv
			}
		}

		if !hasConflict {
			row := colMapToRow(mergedRow, mergedHeaders)
			mergedRows = append(mergedRows, row)
		}
	}

	if hasConflict {
		return nil, HasConflict, nil
	}

	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.Write(mergedHeaders)
	_ = w.WriteAll(mergedRows)
	w.Flush()

	return buf.Bytes(), Merged, nil
}

func parseCSV(data []byte) ([][]string, []string, error) {
	r := csv.NewReader(bytes.NewReader(data))
	records, err := r.ReadAll()
	if err != nil || len(records) == 0 {
		return nil, nil, err
	}
	return records[1:], records[0], nil
}

func indexByFirstCol(rows [][]string) map[string][]string {
	m := make(map[string][]string)
	for _, row := range rows {
		if len(row) > 0 {
			m[row[0]] = row
		}
	}
	return m
}

func toColMap(row []string, headers []string) map[string]string {
	m := make(map[string]string)
	for i, h := range headers {
		if i < len(row) {
			m[h] = row[i]
		}
	}
	return m
}

func colMapToRow(m map[string]string, headers []string) []string {
	row := make([]string, len(headers))
	for i, h := range headers {
		row[i] = m[h]
	}
	return row
}

func mergeHeaders(sets ...[]string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, s := range sets {
		for _, h := range s {
			if !seen[h] {
				out = append(out, h)
				seen[h] = true
			}
		}
	}
	return out
}

func rowsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func concatCSV(localRows, remoteRows [][]string, localH, remoteH []string) ([]byte, Result, error) {
	headers := mergeHeaders(localH, remoteH)
	seenFirst := make(map[string]bool)
	var all [][]string
	for _, row := range localRows {
		all = append(all, row)
		if len(row) > 0 {
			seenFirst[row[0]] = true
		}
	}
	for _, row := range remoteRows {
		if len(row) == 0 || !seenFirst[row[0]] {
			all = append(all, row)
		}
	}
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.Write(headers)
	_ = w.WriteAll(all)
	w.Flush()
	return buf.Bytes(), Merged, nil
}

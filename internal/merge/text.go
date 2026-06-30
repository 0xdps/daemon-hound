package merge

import (
	"bytes"
	"strings"
)

// TextDriver performs line-based 3-way merge (diff3 style) for plain text files.
// Works for .txt, .md, shell scripts, YAML, TOML, and other line-oriented formats.
// Uses LCS to find common anchors between base, local, and remote, then merges hunks.
type TextDriver struct{}

func (d *TextDriver) CanHandle(filename string) bool {
	// Reject known binary extensions — they cannot be line-merged.
	binary := []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".ico",
		".pdf", ".zip", ".tar", ".gz", ".exe", ".bin", ".so", ".dylib"}
	for _, ext := range binary {
		if strings.HasSuffix(filename, ext) {
			return false // binary file — let caller handle as Unsupported
		}
	}
	return true // catch-all for text files
}

func (d *TextDriver) Merge(base, local, remote []byte) ([]byte, Result, error) {
	baseLines := splitLines(base)
	localLines := splitLines(local)
	remoteLines := splitLines(remote)

	merged, ok := merge3Lines(baseLines, localLines, remoteLines)
	if !ok {
		return nil, HasConflict, nil
	}

	return []byte(strings.Join(merged, "\n")), Merged, nil
}

// merge3Lines implements the standard diff3 algorithm on string slices.
// Returns (merged lines, true) on clean merge or (nil, false) on conflict.
func merge3Lines(base, a, b []string) ([]string, bool) {
	editsA := computeEdits(base, a)
	editsB := computeEdits(base, b)

	var result []string
	hasConflict := false

	pos := 0 // current position in base
	iA := 0  // index into editsA
	iB := 0  // index into editsB

	for pos < len(base) || iA < len(editsA) || iB < len(editsB) {
		// Find the next edit boundary from either side
		nextA := len(base)
		nextB := len(base)
		if iA < len(editsA) {
			nextA = editsA[iA].baseFrom
		}
		if iB < len(editsB) {
			nextB = editsB[iB].baseFrom
		}

		// Emit unchanged base lines up to the nearest next edit
		emitTo := nextA
		if nextB < emitTo {
			emitTo = nextB
		}
		result = append(result, base[pos:emitTo]...)
		pos = emitTo

		aActive := iA < len(editsA) && editsA[iA].baseFrom == pos
		bActive := iB < len(editsB) && editsB[iB].baseFrom == pos

		if !aActive && !bActive {
			// Advance past any remaining base lines if no more edits
			if pos < len(base) {
				result = append(result, base[pos])
				pos++
			}
			continue
		}

		if aActive && !bActive {
			result = append(result, editsA[iA].newLines...)
			pos = editsA[iA].baseTo
			iA++
		} else if bActive && !aActive {
			result = append(result, editsB[iB].newLines...)
			pos = editsB[iB].baseTo
			iB++
		} else {
			// Both sides have an edit at this position
			ea, eb := editsA[iA], editsB[iB]
			sameRange := ea.baseTo == eb.baseTo
			sameContent := linesEqual(ea.newLines, eb.newLines)

			if sameRange && sameContent {
				// Identical edit on both sides — emit once
				result = append(result, ea.newLines...)
				pos = ea.baseTo
			} else {
				// True conflict — record it and continue (caller will see conflict)
				hasConflict = true
				// Advance past both edits
				if ea.baseTo > eb.baseTo {
					pos = ea.baseTo
				} else {
					pos = eb.baseTo
				}
			}
			iA++
			iB++
		}
	}

	if hasConflict {
		return nil, false
	}
	return result, true
}

// edit represents a change to a region of the base file.
type edit struct {
	baseFrom int      // first line index in base (inclusive)
	baseTo   int      // last line index in base (exclusive)
	newLines []string // replacement lines
}

// computeEdits returns the set of edits to transform base into changed.
// Uses LCS to find unchanged anchors; everything between them is an edit.
func computeEdits(base, changed []string) []edit {
	matches := lcsIndices(base, changed)

	var edits []edit
	prevBase := 0
	prevChanged := 0

	for _, m := range matches {
		bi, ci := m[0], m[1]
		if bi > prevBase || ci > prevChanged {
			edits = append(edits, edit{
				baseFrom: prevBase,
				baseTo:   bi,
				newLines: changed[prevChanged:ci],
			})
		}
		prevBase = bi + 1
		prevChanged = ci + 1
	}

	if prevBase < len(base) || prevChanged < len(changed) {
		edits = append(edits, edit{
			baseFrom: prevBase,
			baseTo:   len(base),
			newLines: changed[prevChanged:],
		})
	}
	return edits
}

// lcsIndices computes the longest common subsequence of two string slices
// and returns pairs of matching indices [baseIdx, changedIdx].
func lcsIndices(xs, ys []string) [][2]int {
	n, m := len(xs), len(ys)
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			if xs[i-1] == ys[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else if dp[i-1][j] > dp[i][j-1] {
				dp[i][j] = dp[i-1][j]
			} else {
				dp[i][j] = dp[i][j-1]
			}
		}
	}

	var pairs [][2]int
	i, j := n, m
	for i > 0 && j > 0 {
		if xs[i-1] == ys[j-1] {
			pairs = append(pairs, [2]int{i - 1, j - 1})
			i--
			j--
		} else if dp[i-1][j] > dp[i][j-1] {
			i--
		} else {
			j--
		}
	}
	// Reverse to forward order
	for lo, hi := 0, len(pairs)-1; lo < hi; lo, hi = lo+1, hi-1 {
		pairs[lo], pairs[hi] = pairs[hi], pairs[lo]
	}
	return pairs
}

func splitLines(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	lines := strings.Split(string(data), "\n")
	// Remove trailing empty string from final newline
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func linesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Suppress unused import warning
var _ = bytes.Equal

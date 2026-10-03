package sysutil

import (
	"fmt"
	"strings"
)

// LineDiff renders a line diff from a to b: "-" lines only in a, "+" lines
// only in b, and up to two unchanged lines of context around each change.
func LineDiff(a, b string) string {
	x := strings.Split(strings.TrimSuffix(a, "\n"), "\n")
	y := strings.Split(strings.TrimSuffix(b, "\n"), "\n")
	if a == "" {
		x = nil
	}

	// Longest common subsequence table, filled from the end.
	lcs := make([][]int, len(x)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(y)+1)
	}
	for i := len(x) - 1; i >= 0; i-- {
		for j := len(y) - 1; j >= 0; j-- {
			if x[i] == y[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}

	type line struct {
		op   byte
		text string
	}
	var lines []line
	i, j := 0, 0
	for i < len(x) || j < len(y) {
		switch {
		case i < len(x) && j < len(y) && x[i] == y[j]:
			lines = append(lines, line{' ', x[i]})
			i++
			j++
		case i < len(x) && (j == len(y) || lcs[i+1][j] >= lcs[i][j+1]):
			lines = append(lines, line{'-', x[i]})
			i++
		default:
			lines = append(lines, line{'+', y[j]})
			j++
		}
	}

	const contextLines = 2
	var out strings.Builder
	skipped := false
	for k, l := range lines {
		near := false
		for m := max(0, k-contextLines); m <= min(len(lines)-1, k+contextLines); m++ {
			if lines[m].op != ' ' {
				near = true
				break
			}
		}
		if !near {
			skipped = true
			continue
		}
		if skipped {
			out.WriteString("  ...\n")
			skipped = false
		}
		fmt.Fprintf(&out, "%c %s\n", l.op, l.text)
	}
	if out.Len() == 0 {
		return "  (only the trailing newline differs)\n"
	}
	if skipped {
		out.WriteString("  ...\n")
	}
	return out.String()
}

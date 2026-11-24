package cmd

import (
	"bufio"
	"strings"
	"testing"
)

// typeLines feeds the line prompts the given answers, one per line.
func typeLines(t *testing.T, lines ...string) {
	t.Helper()
	saved := stdinScanner
	t.Cleanup(func() { stdinScanner = saved })
	stdinScanner = bufio.NewScanner(strings.NewReader(strings.Join(lines, "\n") + "\n"))
}

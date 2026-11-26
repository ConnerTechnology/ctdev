package cmd

import (
	"bufio"
	"strings"
	"testing"
)

// useStdin feeds prompts the given raw input and restores stdinScanner on cleanup.
func useStdin(t *testing.T, input string) {
	t.Helper()
	saved := stdinScanner
	stdinScanner = bufio.NewScanner(strings.NewReader(input))
	t.Cleanup(func() { stdinScanner = saved })
}

// typeLines feeds the line prompts the given answers, one per line.
func typeLines(t *testing.T, lines ...string) {
	t.Helper()
	useStdin(t, strings.Join(lines, "\n")+"\n")
}

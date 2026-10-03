package sysutil

import "testing"

func TestLineDiff(t *testing.T) {
	for _, tc := range []struct{ name, a, b, want string }{
		{"change", "a\nb\nc\n", "a\nB\nc\n", "  a\n- b\n+ B\n  c\n"},
		{"skips far context", "1\n2\n3\n4\n5\n6\n7\n", "1\n2\n3\nX\n5\n6\n7\n", "  ...\n  2\n  3\n- 4\n+ X\n  5\n  6\n  ...\n"},
		{"trailing newline only", "a\n", "a", "  (only the trailing newline differs)\n"},
	} {
		if got := LineDiff(tc.a, tc.b); got != tc.want {
			t.Errorf("%s: LineDiff =\n%s\nwant\n%s", tc.name, got, tc.want)
		}
	}
}

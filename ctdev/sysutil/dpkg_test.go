package sysutil

import "testing"

func TestParseDebStatusVersion(t *testing.T) {
	for out, want := range map[string]string{
		"installed 1.4.271":              "1.4.271",
		"installed 1:3.52-2\n":           "1:3.52-2",
		"config-files 3.0pl1-184ubuntu2": "",
		"half-configured 1.0":            "",
		"not-installed ":                 "",
		"":                               "",
	} {
		if got := parseDebStatusVersion(out); got != want {
			t.Errorf("parseDebStatusVersion(%q) = %q, want %q", out, got, want)
		}
	}
}

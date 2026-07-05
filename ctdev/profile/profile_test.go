package profile

import (
	"strings"
	"testing"
)

func TestBuiltinsLoadAndAreWellFormed(t *testing.T) {
	list := List()
	if len(list) < 3 {
		t.Fatalf("expected at least the 3 built-in profiles, got %d", len(list))
	}
	for _, want := range []string{"pihole-node", "dev-workstation", "family-desktop"} {
		p, err := Load(want)
		if err != nil {
			t.Fatalf("Load(%q): %v", want, err)
		}
		if p.Description == "" {
			t.Errorf("%s: empty description", want)
		}
		if len(p.Components) == 0 {
			t.Errorf("%s: no components", want)
		}
		if p.Source != "built-in" {
			t.Errorf("%s: source = %q, want built-in", want, p.Source)
		}
	}
}

func TestLoadUnknownProfileListsAvailable(t *testing.T) {
	_, err := Load("no-such-profile")
	if err == nil {
		t.Fatal("expected error for unknown profile")
	}
	if !strings.Contains(err.Error(), "pihole-node") {
		t.Errorf("error should list available profiles, got: %v", err)
	}
}

func TestParseRejectsEmptyProfile(t *testing.T) {
	if _, err := parse("empty", []byte(`description = "nothing here"`), "test"); err == nil {
		t.Error("expected error for profile with no components or categories")
	}
}

func TestParseReadsAllFields(t *testing.T) {
	src := `
description = "test box"
components = ["zsh", "git"]
configure = ["ssh"]
notes = "hello"
`
	p, err := parse("t", []byte(src), "test")
	if err != nil {
		t.Fatal(err)
	}
	if p.Description != "test box" || len(p.Components) != 2 || len(p.Configure) != 1 || p.Notes != "hello" {
		t.Errorf("unexpected parse result: %+v", p)
	}
}

package main

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

func TestSearchNeedsEveryTerm(t *testing.T) {
	sessions := load(t, time.Time{}, day("2100-01-01"))
	terms := []string{"WEEKLY", "report"}
	var out bytes.Buffer
	if n := Search(&out, sessions, terms); n != 2 {
		t.Fatalf("want 2 hits, got %d:\n%s", n, out.String())
	}
	if !strings.HasPrefix(out.String(), "s1 ") {
		t.Errorf("newest session first:\n%s", out.String())
	}
	if terms[0] != "WEEKLY" {
		t.Error("Search changed the caller's terms")
	}
	out.Reset()
	if n := Search(&out, sessions, []string{"make test"}); n == 0 {
		t.Error("tool input isn't searched")
	}
	out.Reset()
	if n := Search(&out, sessions, []string{"weekly", "absent"}); n != 0 {
		t.Fatalf("want no hits, got %d", n)
	}
}

func TestSnippetKeepsCharactersWhole(t *testing.T) {
	text := strings.Repeat("é", 150) + "ÜBER" + strings.Repeat("ü", 150)
	got := snippet(text, "über")
	if !strings.Contains(got, "ÜBER") || !strings.HasPrefix(got, "...é") {
		t.Fatalf("snippet %q", got)
	}
}

func TestSearchSkipsSkillBodies(t *testing.T) {
	sessions := load(t, time.Time{}, day("2100-01-01"))
	var out bytes.Buffer
	if n := Search(&out, sessions, []string{"example skill"}); n != 0 {
		t.Errorf("a skill's body was searched:\n%s", out.String())
	}
}

func TestSearchCapsHits(t *testing.T) {
	var lines []string
	for i := range maxHits + 5 {
		lines = append(lines, bash(fmt.Sprintf("2026-01-02T09:%02d:00Z", i), "echo example", "success", "x"))
	}
	store := mapStore{"refs/entire/checkpoints/h8/s": checkpointFiles(meta("s", "s6", "2026-01-02T10:00:00Z", 1, 0, "a.go"), strings.Join(lines, "\n"))}
	sessions, err := LoadSessions(store, time.Time{}, day("2100-01-01"), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if n := Search(&out, sessions, []string{"echo"}); n != maxHits {
		t.Errorf("want %d hits, got %d", maxHits, n)
	}
}

func TestRunSearchExitCodes(t *testing.T) {
	now := day("2026-01-03")
	if code := run([]string{"search", "weekly"}, fixture(), now); code != 0 {
		t.Errorf("a match: want exit 0, got %d", code)
	}
	if code := run([]string{"search", "absent-example-term"}, fixture(), now); code != 1 {
		t.Errorf("no match: want exit 1, got %d", code)
	}
	if code := run([]string{"search", "--since", "2026-01-02", "old", "session"}, fixture(), now); code != 1 {
		t.Errorf("--since leaves the old session out: want exit 1, got %d", code)
	}
}

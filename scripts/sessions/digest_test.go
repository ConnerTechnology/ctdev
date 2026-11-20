package main

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

type mapStore map[string]map[string]string

func (inst mapStore) Refs(prefix string) ([]string, error) {
	var out []string
	for ref := range inst {
		if strings.HasPrefix(ref, prefix) {
			out = append(out, ref)
		}
	}
	return out, nil
}

func (inst mapStore) Read(ref, file string) ([]byte, error) {
	if s, ok := inst[ref][file]; ok {
		return []byte(s), nil
	}
	return nil, fmt.Errorf("no %s:%s", ref, file)
}

const topMeta = `{"sessions":[{"metadata":"/0/metadata.json","compact_transcript":"/0/transcript.jsonl"}]}`

func meta(cp, session, at string, cacheRead, subagentRead int, file string) string {
	return fmt.Sprintf(`{"checkpoint_id":%q,"session_id":%q,"created_at":%q,"branch":"goal/example","model":"example-model",
"files_touched":[%q],"session_token_usage":{"input_tokens":1,"cache_creation_tokens":0,"cache_read_tokens":%d,"output_tokens":9,"api_call_count":3,
"subagent_tokens":{"cache_read_tokens":%d}}}`, cp, session, at, file, cacheRead, subagentRead)
}

func user(ts, text string) string {
	return fmt.Sprintf(`{"type":"user","ts":%q,"content":[{"text":%q}]}`, ts, text)
}

func tool(ts, name, key, value, status, output string) string {
	return fmt.Sprintf(`{"type":"assistant","ts":%q,"content":[{"type":"tool_use","name":%q,"input":{%q:%q},"result":{"status":%q,"output":%q}}]}`,
		ts, name, key, value, status, output)
}

func bash(ts, cmd, status, output string) string {
	return tool(ts, "Bash", "command", cmd, status, output)
}

// A skill's body as the harness injects it: its wording would pass for a correction.
const exampleSkill = "Base directory for this skill: /home/example/repo/.claude/skills/example-skill\n\n" +
	"# Example skill\n\nDon't skip a step. No, stop and undo it instead."

func checkpointFiles(m, transcript string) map[string]string {
	return map[string]string{"metadata.json": topMeta, "0/metadata.json": m, "0/transcript.jsonl": transcript}
}

// s1 runs on 2026-01-01 and 2026-01-02; s0 is old; one ref is broken.
func fixture() mapStore {
	day1 := []string{
		user("2026-01-01T22:00:00Z", "Start the example report"),
		bash("2026-01-01T22:01:00Z", "ls", "success", "a.go"),
	}
	day2 := append(append([]string{}, day1...),
		user("2026-01-02T10:00:00Z", "Add the example report"),
		bash("2026-01-02T10:01:00Z", "make test", "error", "Exit code 1\nFAIL example"),
		bash("2026-01-02T10:02:00Z", "make test", "error", "Exit code 1\nFAIL example"),
		bash("2026-01-02T10:03:00Z", "make test", "success", strings.Repeat("x", largeOutput)),
		user("2026-01-02T10:04:00Z", "<task-notification>done</task-notification>"),
		user("2026-01-02T10:05:00Z", "No, I said the weekly report"),
		user("2026-01-02T10:06:00Z", "<command-name>/retro</command-name>"),
		user("2026-01-02T10:06:30Z", exampleSkill),
		user("2026-01-02T10:06:40Z", " \n"),
	)
	return mapStore{
		"refs/entire/checkpoints/a1/day1": checkpointFiles(meta("day1", "s1", "2026-01-01T22:02:00Z", 1000000, 0, "a.go"), strings.Join(day1, "\n")),
		"refs/entire/checkpoints/b2/day2": checkpointFiles(meta("day2", "s1", "2026-01-02T10:07:00Z", 5000000, 2000000, "b.go"), strings.Join(day2, "\n")),
		"refs/entire/checkpoints/c3/old": checkpointFiles(meta("old", "s0", "2025-12-01T10:00:00Z", 1, 0, "c.go"),
			user("2025-12-01T10:00:00Z", "An old session about the weekly report")),
		"refs/entire/checkpoints/d4/broken": {"metadata.json": "{not json"},
	}
}

func day(s string) time.Time { t, _ := time.Parse("2006-01-02", s); return t }

func load(t *testing.T, since, until time.Time) []*Session {
	t.Helper()
	sessions, err := LoadSessions(fixture(), since, until, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	return sessions
}

func TestLoadCountsOnlyTheWindow(t *testing.T) {
	sessions := load(t, day("2026-01-02"), day("2026-01-03"))
	if len(sessions) != 1 || sessions[0].CheckpointID != "day2" {
		t.Fatalf("want only checkpoint day2, got %+v", sessions)
	}
	s := sessions[0]
	if s.SessionUsage.CacheRead != 4000000 || s.SessionUsage.Calls != 0 {
		t.Errorf("tokens before the window count: %+v", s.SessionUsage)
	}
	if s.SessionUsage.Subagents.CacheRead != 2000000 {
		t.Errorf("subagent tokens: %+v", s.SessionUsage.Subagents)
	}
	if got := len(s.Prompts()); got != 4 {
		t.Errorf("want 4 prompts in the window (no notification, no empty turn, no day-1 prompt; the slash command and the skill), got %d", got)
	}
	if fmt.Sprint(s.FilesTouched) != "[b.go]" {
		t.Errorf("files from the window only: %v", s.FilesTouched)
	}

	day1 := load(t, day("2026-01-01"), day("2026-01-02"))
	if len(day1) != 1 || day1[0].CheckpointID != "day1" || len(day1[0].Prompts()) != 1 {
		t.Fatalf("rerun of day 1 keeps its own checkpoint: %+v", day1)
	}
}

func TestLoadSkipsBrokenCheckpoints(t *testing.T) {
	var warn bytes.Buffer
	sessions, err := LoadSessions(fixture(), time.Time{}, day("2100-01-01"), &warn)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Errorf("want s0 and s1, got %d", len(sessions))
	}
	if !strings.Contains(warn.String(), "skipping refs/entire/checkpoints/d4/broken") {
		t.Errorf("no warning for the broken ref: %q", warn.String())
	}
}

func TestDigestSignals(t *testing.T) {
	var out bytes.Buffer
	WriteDigest(&out, load(t, day("2026-01-02"), day("2026-01-03")), day("2026-01-02"), day("2026-01-03"))
	for _, want := range []string{
		"| s1 | goal/example | example-model | 4.0M | 4.0M | 0.0M | 0 | 2.0M |",
		"All sessions: 6.0M tokens (2.0M in subagents)",
		"`correction?`: No, I said the weekly report",
		"2x Bash: Exit code 1: FAIL example (e.g. `make test`)",
		"3x `make test`",
		"1x Bash: make test (largest 15000 chars)",
		"skill: example-skill\n",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("digest lacks %q:\n%s", want, out.String())
		}
	}
	for _, unwanted := range []string{"task-notification", "Start the example report", "Base directory", "Example skill"} {
		if strings.Contains(out.String(), unwanted) {
			t.Errorf("digest has %q:\n%s", unwanted, out.String())
		}
	}
}

func TestDigestRepeatedReadsAndLongStretches(t *testing.T) {
	lines := []string{user("2026-01-02T09:00:00Z", "Fix the example")}
	for i := range longStretch {
		lines = append(lines, tool(fmt.Sprintf("2026-01-02T09:%02d:00Z", i+1), "Read", "file_path", "/home/example/repo/a.go", "success", "x"))
	}
	store := mapStore{"refs/entire/checkpoints/e5/s": checkpointFiles(meta("s", "s9", "2026-01-02T10:00:00Z", 1, 0, "a.go"), strings.Join(lines, "\n"))}
	sessions, err := LoadSessions(store, day("2026-01-02"), day("2026-01-03"), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	WriteDigest(&out, sessions, day("2026-01-02"), day("2026-01-03"))
	for _, want := range []string{
		fmt.Sprintf("- /home/example/repo/a.go read %d times in s9", longStretch),
		fmt.Sprintf("- s9: %d tool calls over 30m0s after \"Fix the example\"", longStretch),
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("digest lacks %q:\n%s", want, out.String())
		}
	}
}

func TestDigestEmptyWindow(t *testing.T) {
	var out bytes.Buffer
	WriteDigest(&out, nil, day("2026-02-01"), day("2026-02-02"))
	if !strings.Contains(out.String(), "No checkpointed sessions") {
		t.Fatal(out.String())
	}
}

func TestRunUsage(t *testing.T) {
	now := day("2026-01-03")
	for _, args := range [][]string{nil, {"nope"}, {"search"}, {"digest", "--since", "Jan 2"}} {
		if code := run(args, fixture(), now); code != 2 {
			t.Errorf("%v: want exit 2, got %d", args, code)
		}
	}
	if code := run([]string{"digest"}, fixture(), now); code != 0 {
		t.Errorf("digest: want 0, got %d", code)
	}
}

func TestDigestSaysItIsData(t *testing.T) {
	for _, sessions := range [][]*Session{nil, load(t, day("2026-01-02"), day("2026-01-03"))} {
		var out bytes.Buffer
		WriteDigest(&out, sessions, day("2026-01-02"), day("2026-01-03"))
		head := strings.SplitN(out.String(), "\n", 4)
		if len(head) < 3 || head[2] != dataNotice {
			t.Errorf("the notice isn't the line under the title:\n%s", out.String())
		}
	}
}

func TestSkillIsNamedNotMarkedAsCorrection(t *testing.T) {
	var out bytes.Buffer
	WriteDigest(&out, load(t, day("2026-01-02"), day("2026-01-03")), day("2026-01-02"), day("2026-01-03"))
	for _, line := range strings.Split(out.String(), "\n") {
		if strings.Contains(line, "skill: example-skill") && strings.Contains(line, "correction?") {
			t.Errorf("a skill's body is marked as a correction: %q", line)
		}
	}
	if got := strings.Count(out.String(), "`correction?`"); got != 2 {
		// The legend names the mark once; the one typed correction is the other.
		t.Errorf("want the legend and one correction, got %d marks:\n%s", got, out.String())
	}
}

func TestEntrySkill(t *testing.T) {
	for _, c := range []struct{ kind, text, want string }{
		{"user", exampleSkill, "example-skill"},
		{"user", "  Base directory for this skill: /home/example/skills/tdd/\n\nbody", "tdd"},
		{"user", "Base directory for this skill:\n", "unknown"},
		{"user", "What does Base directory for this skill: /home/example/x mean?", ""},
		{"user", "Fix the example", ""},
		{"assistant", exampleSkill, ""},
	} {
		e := Entry{Type: c.kind, Content: []Block{{Text: c.text}}}
		if got := e.Skill(); got != c.want {
			t.Errorf("%s %q: want %q, got %q", c.kind, c.text, c.want, got)
		}
	}
}

func TestEmptyPromptsAreSkipped(t *testing.T) {
	lines := []string{user("2026-01-02T09:00:00Z", ""), user("2026-01-02T09:01:00Z", "  \n "),
		`{"type":"user","ts":"2026-01-02T09:02:00Z","content":[]}`, user("2026-01-02T09:03:00Z", "Fix the example")}
	store := mapStore{"refs/entire/checkpoints/f6/s": checkpointFiles(meta("s", "s8", "2026-01-02T10:00:00Z", 1, 0, "a.go"), strings.Join(lines, "\n"))}
	sessions, err := LoadSessions(store, day("2026-01-02"), day("2026-01-03"), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if got := sessions[0].Prompts(); len(got) != 1 || got[0].Content[0].Text != "Fix the example" {
		t.Errorf("want only the typed prompt, got %+v", got)
	}
}

// A skill the agent loads mid-task is no new prompt: the stretch goes on, credited to the prompt
// the person typed.
func TestSkillDoesNotEndAStretch(t *testing.T) {
	lines := []string{user("2026-01-02T09:00:00Z", "Fix the example")}
	for i := range longStretch {
		if i == longStretch/2 {
			lines = append(lines, user(fmt.Sprintf("2026-01-02T09:%02d:30Z", i), exampleSkill))
		}
		lines = append(lines, bash(fmt.Sprintf("2026-01-02T09:%02d:00Z", i+1), fmt.Sprintf("echo %d", i), "success", "x"))
	}
	store := mapStore{"refs/entire/checkpoints/g7/s": checkpointFiles(meta("s", "s7", "2026-01-02T10:00:00Z", 1, 0, "a.go"), strings.Join(lines, "\n"))}
	sessions, err := LoadSessions(store, day("2026-01-02"), day("2026-01-03"), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	got := longStretches(sessions[0])
	if len(got) != 1 || !strings.Contains(got[0], fmt.Sprintf("s7: %d tool calls", longStretch)) || !strings.Contains(got[0], `"Fix the example"`) {
		t.Errorf("want one stretch after the typed prompt, got %q", got)
	}
}

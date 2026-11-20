package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"path"
	"sort"
	"strings"
	"time"
)

// Store reads files from git; the real one shells out, tests use a map.
type Store interface {
	Refs(prefix string) ([]string, error)
	Read(ref, file string) ([]byte, error)
}

type gitStore struct{ dir string }

func (inst gitStore) Refs(prefix string) ([]string, error) {
	out, err := exec.Command("git", "-C", inst.dir, "for-each-ref", "--format=%(refname)", prefix).Output()
	if err != nil {
		return nil, fmt.Errorf("git for-each-ref: %w", err)
	}
	return strings.Fields(string(out)), nil
}

func (inst gitStore) Read(ref, file string) ([]byte, error) {
	out, err := exec.Command("git", "-C", inst.dir, "show", ref+":"+file).Output()
	if err != nil {
		return nil, fmt.Errorf("git show %s:%s: %w", ref, file, err)
	}
	return out, nil
}

const checkpointRefs = "refs/entire/checkpoints/"

type Usage struct {
	Input      int `json:"input_tokens"`
	CacheWrite int `json:"cache_creation_tokens"`
	CacheRead  int `json:"cache_read_tokens"`
	Output     int `json:"output_tokens"`
	Calls      int `json:"api_call_count"`
}

func (inst Usage) Total() int { return inst.Input + inst.CacheWrite + inst.CacheRead + inst.Output }

func (inst Usage) Plus(o Usage) Usage {
	return Usage{inst.Input + o.Input, inst.CacheWrite + o.CacheWrite, inst.CacheRead + o.CacheRead, inst.Output + o.Output, inst.Calls + o.Calls}
}

func (inst Usage) Minus(o Usage) Usage {
	return Usage{inst.Input - o.Input, inst.CacheWrite - o.CacheWrite, inst.CacheRead - o.CacheRead, inst.Output - o.Output, inst.Calls - o.Calls}
}

// SessionUsage is a session's running total; Entire counts its subagents apart.
type SessionUsage struct {
	Usage
	Subagents Usage `json:"subagent_tokens"`
}

func (inst SessionUsage) Minus(o SessionUsage) SessionUsage {
	return SessionUsage{inst.Usage.Minus(o.Usage), inst.Subagents.Minus(o.Subagents)}
}

type checkpointMeta struct {
	Sessions []struct {
		Metadata          string `json:"metadata"`
		CompactTranscript string `json:"compact_transcript"`
	} `json:"sessions"`
}

type sessionMeta struct {
	CheckpointID string       `json:"checkpoint_id"`
	SessionID    string       `json:"session_id"`
	CreatedAt    time.Time    `json:"created_at"`
	Branch       string       `json:"branch"`
	Model        string       `json:"model"`
	FilesTouched []string     `json:"files_touched"`
	SessionUsage SessionUsage `json:"session_token_usage"`
}

type checkpoint struct {
	sessionMeta
	ref, transcript string
}

// Session is what one agent session did inside the window: its prompts and tool calls from the
// window, and the tokens it spent there (its running total at its last checkpoint in the window,
// less the total at its last checkpoint before it).
type Session struct {
	sessionMeta
	entries []Entry
}

// Entry is one line of Entire's compact transcript.
type Entry struct {
	Type    string    `json:"type"`
	TS      time.Time `json:"ts"`
	Content []Block   `json:"content"`
}

type Block struct {
	Type   string          `json:"type"`
	Text   string          `json:"text"`
	Name   string          `json:"name"`
	Input  json.RawMessage `json:"input"`
	Result *struct {
		Status string `json:"status"`
		Output string `json:"output"`
	} `json:"result"`
}

// LoadSessions returns the sessions with a checkpoint made in [since, until). A checkpoint that
// can't be read is skipped with a warning, so one bad ref never stops the daily retro.
func LoadSessions(store Store, since, until time.Time, warn io.Writer) ([]*Session, error) {
	refs, err := store.Refs(checkpointRefs)
	if err != nil {
		return nil, err
	}
	bySession := map[string][]checkpoint{}
	for _, ref := range refs {
		cps, err := readCheckpoint(store, ref)
		if err != nil {
			fmt.Fprintf(warn, "sessions: skipping %s: %v\n", ref, err)
			continue
		}
		for _, cp := range cps {
			bySession[cp.SessionID] = append(bySession[cp.SessionID], cp)
		}
	}
	var out []*Session
	for _, cps := range bySession {
		sort.Slice(cps, func(i, j int) bool { return cps[i].CreatedAt.Before(cps[j].CreatedAt) })
		var before SessionUsage
		var last *checkpoint
		files := map[string]bool{}
		for i := range cps {
			switch {
			case cps[i].CreatedAt.Before(since):
				before = cps[i].SessionUsage
			case cps[i].CreatedAt.Before(until):
				last = &cps[i]
				for _, f := range cps[i].FilesTouched {
					files[f] = true
				}
			}
		}
		if last == nil {
			continue
		}
		raw, err := store.Read(last.ref, last.transcript)
		var entries []Entry
		if err == nil {
			entries, err = parseTranscript(raw)
		}
		if err != nil {
			fmt.Fprintf(warn, "sessions: skipping %s %s: %v\n", last.ref, last.transcript, err)
			continue
		}
		s := &Session{sessionMeta: last.sessionMeta}
		s.SessionUsage = last.SessionUsage.Minus(before)
		s.FilesTouched = s.FilesTouched[:0:0]
		for f := range files {
			s.FilesTouched = append(s.FilesTouched, f)
		}
		sort.Strings(s.FilesTouched)
		for _, e := range entries {
			if !e.TS.Before(since) && e.TS.Before(until) {
				s.entries = append(s.entries, e)
			}
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func readCheckpoint(store Store, ref string) ([]checkpoint, error) {
	raw, err := store.Read(ref, "metadata.json")
	if err != nil {
		return nil, err
	}
	var cp checkpointMeta
	if err := json.Unmarshal(raw, &cp); err != nil {
		return nil, fmt.Errorf("metadata.json: %w", err)
	}
	var out []checkpoint
	for _, s := range cp.Sessions {
		file := strings.TrimPrefix(s.Metadata, "/")
		raw, err := store.Read(ref, file)
		if err != nil {
			return nil, err
		}
		var m sessionMeta
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		transcript := strings.TrimPrefix(s.CompactTranscript, "/")
		if transcript == "" {
			transcript = path.Join(path.Dir(file), "transcript.jsonl")
		}
		out = append(out, checkpoint{sessionMeta: m, ref: ref, transcript: transcript})
	}
	return out, nil
}

func parseTranscript(raw []byte) ([]Entry, error) {
	var out []Entry
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var e Entry
		if err := json.Unmarshal(line, &e); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, sc.Err()
}

// Prompts are the user turns in order: what the person typed, and the skill bodies the harness
// injects as user turns (Skill names those). Harness notifications and empty turns are left out.
func (inst *Session) Prompts() []Entry {
	var out []Entry
	for _, e := range inst.entries {
		if e.isPrompt() {
			out = append(out, e)
		}
	}
	return out
}

func (inst Entry) isPrompt() bool {
	if inst.Type != "user" || len(inst.Content) == 0 {
		return false
	}
	text := strings.TrimSpace(inst.Content[0].Text)
	return text != "" && !isNotification(text)
}

// A skill's body reaches the transcript as a user turn that starts with this line, whether a
// slash command or the agent's Skill tool loaded it. The compact transcript keeps no isMeta flag
// (only full.jsonl does), so the text is the one test it allows.
const skillPrefix = "Base directory for this skill:"

// Skill is the name of the skill whose body this user turn is (the last component of its base
// directory), or "" when the person typed it.
func (inst Entry) Skill() string {
	if inst.Type != "user" || len(inst.Content) == 0 {
		return ""
	}
	rest, ok := strings.CutPrefix(strings.TrimSpace(inst.Content[0].Text), skillPrefix)
	if !ok {
		return ""
	}
	dir := strings.TrimRight(strings.TrimSpace(firstLine(rest)), "/")
	if dir == "" {
		return "unknown"
	}
	return path.Base(dir)
}

// typed is a prompt the person wrote, the only kind that starts a new stretch of work.
func (inst Entry) typed() bool { return inst.isPrompt() && inst.Skill() == "" }

// Messages the harness adds as user turns. A slash command (<command-name>) is a real prompt.
var notificationPrefixes = []string{"<task-notification>", "<system-reminder>", "<local-command-stdout>",
	"<local-command-stderr>", "<local-command-caveat>", "<bash-stdout>", "<bash-stderr>"}

func isNotification(text string) bool {
	text = strings.TrimSpace(text)
	for _, p := range notificationPrefixes {
		if strings.HasPrefix(text, p) {
			return true
		}
	}
	return false
}

// ToolCalls returns every tool call in order.
func (inst *Session) ToolCalls() []Block {
	var out []Block
	for _, e := range inst.entries {
		if e.Type != "assistant" {
			continue
		}
		for _, b := range e.Content {
			if b.Name != "" {
				out = append(out, b)
			}
		}
	}
	return out
}

// Target is what a tool call acted on: the command, the file or the query.
func (inst Block) Target() string {
	var in map[string]any
	_ = json.Unmarshal(inst.Input, &in)
	for _, k := range []string{"command", "file_path", "pattern", "query", "url", "prompt", "description"} {
		if v, ok := in[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func (inst Block) Failed() bool { return inst.Result != nil && inst.Result.Status == "error" }

func (inst Block) OutputLen() int {
	if inst.Result == nil {
		return 0
	}
	return len(inst.Result.Output)
}

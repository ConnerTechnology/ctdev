// Package claudeclitest puts a fake `claude` first on PATH for tests, so
// nothing a test does reaches the real Claude Code or its config.
package claudeclitest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// script logs each call's arguments, one call per line, to $FAKE_CLAUDE_LOG
// and answers from files in $FAKE_CLAUDE_DIR named after the subcommand
// ("plugin-install.out", "plugin-install.exit"), so a test can script a reply.
const script = `#!/bin/sh
printf '%s\n' "$*" >> "$FAKE_CLAUDE_LOG"
key=$(printf '%s-%s' "$1" "$2")
if [ "$1 $2" = "plugin marketplace" ]; then key=plugin-marketplace-$3; fi
if [ -f "$FAKE_CLAUDE_DIR/$key.out" ]; then cat "$FAKE_CLAUDE_DIR/$key.out"; fi
if [ -f "$FAKE_CLAUDE_DIR/$key.exit" ]; then exit "$(cat "$FAKE_CLAUDE_DIR/$key.exit")"; fi
exit 0
`

// Fake is one fake claude on PATH.
type Fake struct {
	t   *testing.T
	dir string
	log string
}

// New installs a fake claude for the test and points CLAUDE_CONFIG_DIR at a
// temp dir.
func New(t *testing.T) *Fake {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	f := &Fake{t: t, dir: dir, log: filepath.Join(dir, "calls.log")}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_CLAUDE_LOG", f.log)
	t.Setenv("FAKE_CLAUDE_DIR", dir)
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(dir, "config"))
	return f
}

// Reply sets what a subcommand prints and its exit code. key is the first two
// arguments joined by a dash ("plugin-install", "mcp-add-json"), or
// "plugin-marketplace-add" for marketplace subcommands.
func (f *Fake) Reply(key, stdout string, exit int) {
	f.t.Helper()
	if err := os.WriteFile(filepath.Join(f.dir, key+".out"), []byte(stdout), 0o644); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.dir, key+".exit"), []byte(strconv.Itoa(exit)), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

// Calls lists each call's arguments, joined by spaces, in order.
func (f *Fake) Calls() []string {
	f.t.Helper()
	data, err := os.ReadFile(f.log)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		f.t.Fatal(err)
	}
	return strings.Split(strings.TrimRight(string(data), "\n"), "\n")
}

// SetLocalMCPServer writes a local-scope server for root into the fake config
// file, as Claude Code would after `claude mcp add-json`.
func (f *Fake) SetLocalMCPServer(root, name, entry string) {
	f.t.Helper()
	dir := os.Getenv("CLAUDE_CONFIG_DIR")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		f.t.Fatal(err)
	}
	doc := `{"projects":{` + quote(root) + `:{"mcpServers":{` + quote(name) + `:` + entry + `}}}}`
	if err := os.WriteFile(filepath.Join(dir, ".claude.json"), []byte(doc), 0o600); err != nil {
		f.t.Fatal(err)
	}
}

func quote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

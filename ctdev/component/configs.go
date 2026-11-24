package component

import "embed"

// Configs holds the files installers deploy, under configs/<component>/. The
// all: prefix is required so dot-files such as .zshrc are embedded too.
//
// A component with config files installs in two phases. Phase 1 installs the
// software and is skipped when it is already there (unless --force); phase 2
// always deploys the configs with sysutil.DeployFileFromFS, which keeps the
// dotfiles in sync on a re-run. The exception is claude-code, which owns files
// the user also edits (~/.claude/settings.json, CLAUDE.md): its phase 2 writes
// only the missing files, and the review in cmd/claude_code_review.go shows
// the diff and asks before replacing one.
//
//go:embed all:configs
var Configs embed.FS

// Package gitrepo locates the git work tree a command runs in and keeps
// per-machine files out of its history through the repo's info/exclude.
package gitrepo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Repo is a git work tree, located by Root.
type Repo struct {
	Root string
}

// ErrNotARepo means the directory is not inside a git work tree.
var ErrNotARepo = errors.New("not inside a git repository (run this from the repo you want to set up)")

// Find locates the top of the git work tree dir is in. Outside a work tree it
// returns ErrNotARepo; any other failure (git missing, say) comes back as
// itself.
func Find(ctx context.Context, dir string) (Repo, error) {
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			stderr := strings.TrimSpace(string(ee.Stderr))
			if strings.Contains(stderr, "not a git repository") {
				return Repo{}, ErrNotARepo
			}
			if stderr != "" {
				return Repo{}, fmt.Errorf("git rev-parse --show-toplevel: %s", stderr)
			}
		}
		return Repo{}, fmt.Errorf("cannot run git: %w", err)
	}
	root := strings.TrimSpace(string(out))
	if root == "" {
		// A bare repository, or inside .git: there is no work tree to set up.
		return Repo{}, ErrNotARepo
	}
	return Repo{Root: root}, nil
}

// Path joins a repo-relative path onto the root.
func (r Repo) Path(rel string) string {
	return filepath.Join(r.Root, filepath.FromSlash(rel))
}

// MainRoot is the main checkout's work tree: Root itself, except in a linked
// worktree, where it is the checkout that owns the .git directory. Claude Code
// keys a project's local MCP servers by it and keeps its
// .claude/settings.local.json there, so every worktree of a repo shares them.
// When the git directory isn't a work tree's .git (a submodule, say), it is
// Root.
func (r Repo) MainRoot(ctx context.Context) (string, error) {
	out, err := gitOutput(ctx, r.Root, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", fmt.Errorf("cannot find the main checkout: %w", err)
	}
	common := filepath.Clean(strings.TrimSpace(out))
	if filepath.Base(common) != ".git" {
		return r.Root, nil
	}
	return filepath.Dir(common), nil
}

// ExcludeFile is the repo's info/exclude, wherever git keeps it, which in a
// worktree is not under the work tree's own .git.
func (r Repo) ExcludeFile(ctx context.Context) (string, error) {
	out, err := gitOutput(ctx, r.Root, "rev-parse", "--git-path", "info/exclude")
	if err != nil {
		return "", fmt.Errorf("cannot find the git exclude file: %w", err)
	}
	p := strings.TrimSpace(out)
	if !filepath.IsAbs(p) {
		p = filepath.Join(r.Root, p)
	}
	return p, nil
}

// IsIgnored reports whether git ignores a repo-relative path.
func (r Repo) IsIgnored(ctx context.Context, rel string) bool {
	return exec.CommandContext(ctx, "git", "-C", r.Root, "check-ignore", "-q", rel).Run() == nil
}

// IsTracked reports whether a repo-relative path is committed or staged.
func (r Repo) IsTracked(ctx context.Context, rel string) bool {
	return exec.CommandContext(ctx, "git", "-C", r.Root, "ls-files", "--error-unmatch", rel).Run() == nil
}

// AppendExclude adds line to a git exclude file unless it, or the same path
// anchored with a leading slash, is already there. added is false when
// nothing was written.
func AppendExclude(path, line string) (added bool, err error) {
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	bare := strings.TrimPrefix(line, "/")
	for _, l := range strings.Split(string(data), "\n") {
		l = strings.TrimSpace(l)
		if l == bare || l == "/"+bare {
			return false, nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return false, err
	}
	prefix := ""
	if len(data) > 0 && !bytes.HasSuffix(data, []byte("\n")) {
		prefix = "\n"
	}
	if _, err := f.WriteString(prefix + line + "\n"); err != nil {
		f.Close()
		return false, err
	}
	return true, f.Close()
}

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(ee.Stderr)))
		}
		return "", err
	}
	return string(out), nil
}

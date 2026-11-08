---
paths:
  - "ctdev/**/*.go"
---
# Conventions

- Follow standard Go style (Effective Go, Go Code Review Comments). Receiver names are short and the same across a type's methods, so methods added to an existing type use the receiver it already has
- No sleeps for timing in tests: synchronize goroutines with channels and `sync.WaitGroup`
- Every `sysutil` shell-out takes `(ctx context.Context, o sysutil.Opts, ...)` — thread `ctx` from the caller so Ctrl-C cancels in-flight work
- Build `sysutil.Opts` with `execOpts(opts)`, never by hand
- Use `alreadyInstalled("<name>")` for install-time "already present?" checks instead of re-checking paths/commands inline
- For unsupported package-manager branches, return `unsupportedPMError("<component>", p.PackageManager)`
- Config files are embedded via `all:configs` in `go:embed` (the `all:` prefix is required to include dot-files like `.zshrc`)
- Deploy configs with `sysutil.DeployFileFromFS`
- Components with configs use Phase 1/Phase 2: Phase 1 installs the binary (skip if exists), Phase 2 always deploys configs
- Exception: `claude-code` owns files the user also edits (`~/.claude/settings.json`, `CLAUDE.md`), so its Phase 2 writes only missing files and the review in `cmd/configure_claude_code.go` shows the diff and asks before replacing (ADR-0009)
- For `.deb` installs on apt, use `installDebWithDepFix(ctx, o, debPath, pkgName)`; it installs through `apt-get`, not `dpkg -i`
- A component whose install bypasses a package manager and doesn't update itself (a downloaded `.deb`, an upstream `install.sh`, a release tarball) gets a scanner in `cmd/update_scan.go`: it compares the installed version with the vendor's latest stable release, and applying the update re-runs the install. An app that updates itself (a cask marked `auto_updates`) gets no scanner. See "Updates" in `docs/adding-a-component.md`
- All installers accept `ctx context.Context` as first parameter
- Change packages through `sysutil.APTGet` (or `InstallPackage` / `RemovePackage` / `APTUpdate`), never `SudoRun` with `apt-get`, `apt` or `dpkg`: it adds the no-prompt, lock-wait and retry options a boot script with no TTY needs. Read-only queries run as the user (`apt-get -s`, `dpkg -s`/`-l`) are the exception. Install gpg with `sysutil.EnsureGPG`
- Never spell `sudo` in an argv — use `sysutil.SudoRun` (or `sysutil.SudoNoPrompt` for silent probes). They drop the wrapper when we already are root, which is how a container with no sudo installed still works

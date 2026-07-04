package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/ConnerTechnology/dotfiles/ctdev/tui/styles"
	"github.com/charmbracelet/x/term"
)

// This file is the single prompt layer for every interactive wizard (restic,
// caddy, git, aws, the category wizard). All prompts are context-aware so the
// root SIGINT handler cancels them cleanly — a bare stdin read would swallow
// the first Ctrl-C and appear to hang.

var stdinScanner = bufio.NewScanner(os.Stdin)

// errPromptCancelled is returned when the user interrupts a prompt (Ctrl-C,
// which cancels the command context, or EOF). It's mapped to a clean exit, not
// an error — see cancelToClean.
var errPromptCancelled = errors.New("cancelled")

// cancelToClean maps a prompt cancellation to a friendly message and a nil
// error so Ctrl-C out of a wizard exits 0 instead of printing an error.
func cancelToClean(err error) error {
	if errors.Is(err, errPromptCancelled) {
		fmt.Println("\nCancelled — nothing was changed.")
		return nil
	}
	return err
}

// readLineCtx reads one line, but returns as soon as ctx is cancelled (e.g. the
// root SIGINT handler firing on Ctrl-C) so a prompt never blocks an exit. ok is
// false on cancel or EOF.
//
// On cancel the reader goroutine stays blocked on the shared scanner; that's
// fine because a cancelled prompt always unwinds to process exit.
func readLineCtx(ctx context.Context) (line string, ok bool) {
	type res struct {
		line string
		ok   bool
	}
	// Snapshot the scanner on the caller's goroutine: a cancel-leaked reader
	// must not touch the package var concurrently with anyone reassigning it
	// (the race detector flags exactly that in tests that swap stdinScanner).
	scanner := stdinScanner
	ch := make(chan res, 1)
	go func() {
		if scanner.Scan() {
			ch <- res{strings.TrimSpace(scanner.Text()), true}
			return
		}
		ch <- res{"", false}
	}()
	select {
	case <-ctx.Done():
		return "", false
	case r := <-ch:
		return r.line, r.ok
	}
}

// readSecretLineCtx reads one line without echoing it, so credentials never
// land in terminal scrollback, tmux capture buffers, or screen recordings.
// Falls back to a plain (echoed) read when stdin isn't a terminal — pipes and
// tests keep working. ok is false on cancel or EOF.
func readSecretLineCtx(ctx context.Context) (line string, ok bool) {
	fd := os.Stdin.Fd()
	if !term.IsTerminal(fd) {
		return readLineCtx(ctx)
	}
	// Snapshot the termios state: ReadPassword turns echo off and only restores
	// it on its own return path, so a Ctrl-C mid-read would otherwise leave the
	// user's shell with echo disabled.
	saved, err := term.GetState(fd)
	if err != nil {
		return readLineCtx(ctx)
	}

	type res struct {
		line string
		ok   bool
	}
	ch := make(chan res, 1)
	go func() {
		b, err := term.ReadPassword(fd)
		if err != nil {
			ch <- res{"", false}
			return
		}
		ch <- res{strings.TrimSpace(string(b)), true}
	}()
	select {
	case <-ctx.Done():
		_ = term.Restore(fd, saved)
		fmt.Println()
		return "", false
	case r := <-ch:
		fmt.Println() // the swallowed Enter — keep the next prompt on its own line
		return r.line, r.ok
	}
}

// promptWithDefaultCtx prompts with the current value shown in brackets; an
// empty reply keeps the default.
func promptWithDefaultCtx(ctx context.Context, label, def string) (string, error) {
	fmt.Printf("  %s ", styles.Dimmed.Render(fmt.Sprintf("%s [%s]:", label, orDash(def))))
	line, ok := readLineCtx(ctx)
	if !ok {
		return "", errPromptCancelled // Ctrl-C (ctx cancelled) or Ctrl-D (EOF)
	}
	if line != "" {
		return line, nil
	}
	return def, nil
}

// promptRequiredCtx is like promptWithDefaultCtx but re-asks until a value is
// given (or the user cancels). def is offered as the suggested value.
func promptRequiredCtx(ctx context.Context, label, def string) (string, error) {
	for {
		v, err := promptWithDefaultCtx(ctx, label, def)
		if err != nil {
			return "", err
		}
		if v != "" {
			return v, nil
		}
		fmt.Println(styles.Warning.Render("  required — please enter a value"))
	}
}

// promptSecretCtx prompts for a secret with masked input. It never echoes the
// existing value — the hint only signals whether one is already set, and an
// empty reply keeps it.
func promptSecretCtx(ctx context.Context, label, def string) (string, error) {
	hint := ""
	if def != "" {
		hint = " [keep existing]"
	}
	fmt.Printf("  %s ", styles.Dimmed.Render(fmt.Sprintf("%s%s:", label, hint)))
	line, ok := readSecretLineCtx(ctx)
	if !ok {
		return "", errPromptCancelled
	}
	if line != "" {
		return line, nil
	}
	return def, nil
}

// promptSecretRequiredCtx prompts for a secret that must have a value. An empty
// reply keeps current when it's set; otherwise it re-asks. This also shrugs off a
// phantom blank line (e.g. pasting a key whose trailing newline lands as an extra
// Enter) instead of silently recording an empty credential.
func promptSecretRequiredCtx(ctx context.Context, label, current string) (string, error) {
	for {
		v, err := promptSecretCtx(ctx, label, current)
		if err != nil {
			return "", err
		}
		if v != "" {
			return v, nil
		}
		fmt.Println(styles.Warning.Render("  required — please enter a value"))
	}
}

// promptChoiceCtx reads a 1-based numbered-menu selection, returning def on an
// empty or non-numeric reply. The caller prints the menu and prompt.
func promptChoiceCtx(ctx context.Context, def int) (int, error) {
	line, ok := readLineCtx(ctx)
	if !ok {
		return 0, errPromptCancelled
	}
	if line == "" {
		return def, nil
	}
	n, err := strconv.Atoi(line)
	if err != nil {
		return def, nil
	}
	return n, nil
}

// promptYesNoCtx asks a [Y/n] / [y/N] question; an empty reply takes def.
func promptYesNoCtx(ctx context.Context, label string, def bool) (bool, error) {
	hint := "[Y/n]"
	if !def {
		hint = "[y/N]"
	}
	fmt.Printf("  %s %s: ", label, styles.Dimmed.Render(hint))
	line, ok := readLineCtx(ctx)
	if !ok {
		return false, errPromptCancelled
	}
	if line == "" {
		return def, nil
	}
	switch strings.ToLower(line) {
	case "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	default:
		return def, nil
	}
}

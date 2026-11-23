package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/styles"
)

// sshMenuEntries is the interactive `ctdev configure ssh` (CTD-177). A nil
// run is Quit.
var sshMenuEntries = []struct {
	label string
	run   func(context.Context) error
}{
	{"Your keys", yourKeys},
	{"Create a key", createKeyFromMenu},
	{"Server settings", sshServerSettings},
	{"Quit", nil},
}

// runSSHMenu shows the menu until Quit.
func runSSHMenu(ctx context.Context) error {
	for {
		fmt.Println(styles.Title.Render("SSH"))
		for i, e := range sshMenuEntries {
			fmt.Printf("    %d) %s\n", i+1, e.label)
		}
		fmt.Print("  Select [1]: ")
		choice, err := promptChoiceCtx(ctx, 1)
		if err != nil {
			return err
		}
		fmt.Println()
		if choice < 1 || choice > len(sshMenuEntries) {
			fmt.Println(styles.Dimmed.Render("  Invalid choice."))
			continue
		}
		run := sshMenuEntries[choice-1].run
		if run == nil {
			return nil
		}
		if err := run(ctx); err != nil {
			return err
		}
		fmt.Println()
	}
}

// sshServerSettings is the ssh settings category's wizard. Only it needs
// root, so the menu asks for sudo here rather than up front.
func sshServerSettings(ctx context.Context) error {
	if !flagDryRun {
		if err := ensureSudo(ctx); err != nil {
			return fmt.Errorf("sudo required: %w", err)
		}
	}
	return runCategoryWizard(ctx, "ssh", false)
}

// yourKeys lists the key pairs in ~/.ssh, then runs actions on the ones picked.
func yourKeys(ctx context.Context) error {
	pairs := sysutil.FindSSHKeyPairs()
	if len(pairs) == 0 {
		fmt.Println(styles.Dimmed.Render("  No key pairs in ~/.ssh. Choose Create a key to make one."))
		return nil
	}
	fmt.Println(styles.Header.Render("Your keys"))
	for i, p := range pairs {
		fmt.Printf("    %d) %s %s\n", i+1, tildePath(p.Private), styles.Dimmed.Render(strings.TrimSpace(p.Type+"  "+p.Comment)))
	}
	for {
		fmt.Printf("  %s ", styles.Dimmed.Render("Keys, e.g. 1 or 1 3 [back]:"))
		line, ok := readLineCtx(ctx)
		if !ok {
			return errPromptCancelled
		}
		if line == "" {
			return nil
		}
		picks, ok := parsePicks(line, len(pairs))
		if !ok || len(picks) == 0 {
			fmt.Println(styles.Dimmed.Render(fmt.Sprintf("  Enter numbers from 1 to %d.", len(pairs))))
			continue
		}
		var keys []sysutil.SSHKeyPair
		for _, i := range picks {
			keys = append(keys, pairs[i-1])
		}
		fmt.Println()
		return runKeyActions(ctx, keys)
	}
}

// runKeyActions offers the actions for keys until Back, or until they're
// deleted.
func runKeyActions(ctx context.Context, keys []sysutil.SSHKeyPair) error {
	actions := keyActionsFor(len(keys))
	for {
		names := make([]string, len(keys))
		for i, k := range keys {
			names[i] = filepath.Base(k.Private)
		}
		fmt.Println(styles.Header.Render(strings.Join(names, ", ")))
		for i, a := range actions {
			fmt.Printf("    %d) %s\n", i+1, a.label(len(keys)))
		}
		fmt.Printf("    %d) Back\n", len(actions)+1)
		fmt.Print("  Select [1]: ")
		choice, err := promptChoiceCtx(ctx, 1)
		if err != nil {
			return err
		}
		fmt.Println()
		if choice == len(actions)+1 {
			return nil
		}
		if choice < 1 || choice > len(actions) {
			fmt.Println(styles.Dimmed.Render("  Invalid choice."))
			continue
		}
		switch actions[choice-1] {
		case copyToServer:
			err = copyKeysToServer(ctx, keys)
		case uploadToGitHub:
			err = uploadKeys(ctx, keys)
		case copyToClipboard:
			err = copyKeyToClipboard(ctx, keys[0])
		case deleteKeys:
			deleted, derr := deleteKeyPairs(ctx, keys)
			if derr != nil || deleted {
				return derr
			}
		}
		if err != nil {
			return err
		}
		fmt.Println()
	}
}

// copyKeysToServer asks for the server once, then runs ssh-copy-id per key.
func copyKeysToServer(ctx context.Context, keys []sysutil.SSHKeyPair) error {
	target, err := promptRequiredCtx(ctx, "Server (user@host)", "")
	if err != nil {
		return err
	}
	var port string
	for {
		port, err = promptWithDefaultCtx(ctx, "Port (Enter for the default)", "")
		if err != nil {
			return err
		}
		if n, perr := strconv.Atoi(port); port == "" || (perr == nil && n >= 1 && n <= 65535) {
			break
		}
		fmt.Println(styles.Warning.Render("  A port is a number from 1 to 65535."))
	}
	for _, k := range keys {
		args := sshCopyIDArgs(k.Public, target, port)
		if flagDryRun {
			printDryRun("ssh-copy-id", args...)
			continue
		}
		fmt.Printf("  Copying %s to %s...\n", filepath.Base(k.Public), target)
		if err := runTerminalTool(ctx, "ssh-copy-id", args...); err != nil {
			fmt.Println(styles.Warning.Render(fmt.Sprintf("  ssh-copy-id failed for %s: %v", filepath.Base(k.Public), err)))
			continue
		}
		fmt.Println(styles.Success.Render("  Copied " + filepath.Base(k.Public) + " to " + target))
	}
	return nil
}

var githubUses = []struct {
	use   githubKeyUse
	label string
}{
	{githubAuthentication, "Authentication (git push and pull over SSH)"},
	{githubSigning, "Signing (verified commits)"},
	{githubBoth, "Both"},
}

// uploadKeys asks once what the keys are for, then adds each with gh.
func uploadKeys(ctx context.Context, keys []sysutil.SSHKeyPair) error {
	fmt.Println("  What are they for on GitHub?")
	for i, u := range githubUses {
		fmt.Printf("    %d) %s\n", i+1, u.label)
	}
	fmt.Printf("  Select [%d]: ", len(githubUses))
	choice, err := promptChoiceCtx(ctx, len(githubUses))
	if err != nil {
		return err
	}
	if choice < 1 || choice > len(githubUses) {
		fmt.Println(styles.Dimmed.Render("  Invalid choice."))
		return nil
	}
	use := githubUses[choice-1].use
	fmt.Println()

	ready := flagDryRun || githubReady(ctx)
	if !ready {
		fmt.Println(styles.Warning.Render("  gh isn't installed or signed in (gh auth login), so add the keys by hand."))
	}
	hostname, _ := os.Hostname()
	for _, k := range keys {
		title := "ctdev-" + hostname + "-" + filepath.Base(k.Private)
		if ready {
			uploadKeyToGitHub(ctx, k.Public, title, use)
			continue
		}
		if data, err := os.ReadFile(k.Public); err == nil {
			fmt.Printf("\n  %s\n", styles.Dimmed.Render(strings.TrimSpace(string(data))))
		}
		printManualGitHubInstructions(title, use)
	}
	return nil
}

// copyKeyToClipboard puts the public key on the clipboard, and prints it too
// in case OSC 52 doesn't get through.
func copyKeyToClipboard(ctx context.Context, key sysutil.SSHKeyPair) error {
	data, err := os.ReadFile(key.Public)
	if err != nil {
		return err
	}
	text := strings.TrimSpace(string(data))
	tool := clipboardFor(keyGOOS, os.Getenv, hasTool)
	if flagDryRun {
		if tool.name == "" {
			fmt.Println("  [dry-run] would copy the public key to your terminal's clipboard (OSC 52)")
		} else {
			printDryRun(tool.name, tool.args...)
		}
		return nil
	}
	if tool.name != "" {
		if err := runInputTool(ctx, text, tool.name, tool.args...); err == nil {
			fmt.Println(styles.Success.Render("  Copied " + filepath.Base(key.Public) + " to the clipboard"))
			return nil
		}
		fmt.Println(styles.Warning.Render("  " + tool.name + " failed; sending it to the terminal instead."))
	}
	fmt.Fprint(terminalOut, osc52(text))
	fmt.Println(styles.Success.Render("  Sent " + filepath.Base(key.Public) + " to your terminal's clipboard (OSC 52)"))
	fmt.Println(styles.Dimmed.Render("  If it doesn't paste, your terminal blocks OSC 52 (in tmux: set -g set-clipboard on). Here it is:"))
	fmt.Printf("  %s\n", text)
	return nil
}

// deleteKeyPairs removes both files of each key after a confirmation that
// names them. deleted is false when the user said no or under --dry-run.
func deleteKeyPairs(ctx context.Context, keys []sysutil.SSHKeyPair) (deleted bool, err error) {
	fmt.Println("  This deletes:")
	for _, k := range keys {
		fmt.Printf("    %s\n    %s\n", tildePath(k.Private), tildePath(k.Public))
	}
	fmt.Println(styles.Dimmed.Render("  Copies on GitHub and on servers stay; remove those there."))
	if flagDryRun {
		fmt.Println("  [dry-run] would delete these files")
		return false, nil
	}
	yes, err := promptYesNoCtx(ctx, "Delete these files?", false)
	if err != nil || !yes {
		return false, err
	}
	for _, k := range keys {
		for _, p := range []string{k.Private, k.Public} {
			if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
				return false, err
			}
		}
		fmt.Println(styles.Success.Render("  Deleted " + filepath.Base(k.Private)))
	}
	return true, nil
}

// createKeyFromMenu is Create a key: type, file name, comment, then
// ssh-keygen's own passphrase prompt, then the single-key actions.
func createKeyFromMenu(ctx context.Context) error {
	fmt.Println(styles.Header.Render("Create a key"))
	for i, t := range keyTypes {
		note := ""
		if t.note != "" {
			note = styles.Dimmed.Render(" (" + t.note + ")")
		}
		fmt.Printf("    %d) %s%s\n", i+1, t.name, note)
	}
	fmt.Print("  Type [1]: ")
	choice, err := promptChoiceCtx(ctx, 1)
	if err != nil {
		return err
	}
	if choice < 1 || choice > len(keyTypes) {
		fmt.Println(styles.Dimmed.Render("  Invalid choice."))
		return nil
	}
	typ := keyTypes[choice-1]

	dir, err := sshDir()
	if err != nil {
		return err
	}
	path, err := promptKeyPath(ctx, dir, typ)
	if err != nil {
		return err
	}
	comment, err := promptWithDefaultCtx(ctx, "Comment", defaultKeyComment())
	if err != nil {
		return err
	}
	fmt.Println()

	pair, ok, err := createKey(ctx, newKey{typ: typ, path: path, comment: comment})
	if err != nil || !ok {
		return err
	}
	return runKeyActions(ctx, []sysutil.SSHKeyPair{pair})
}

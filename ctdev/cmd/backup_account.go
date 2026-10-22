package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ConnerTechnology/ctdev/ctdev/backup"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/backupscreen"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/styles"
)

// The --all actions on a destination's computers and secrets (CTD-124,
// ADR-0016). Each asks for the master password first and goes through
// backup.Account, which only a verified master password unlocks; a Recovery
// Code reset and pasting an updated kit are the exceptions.

// pasteOnOtherComputers is what to do with a new kit everywhere else. It
// names the menu path that takes it, and the offer --all makes when the
// destination can't be read with the old one.
const pasteOnOtherComputers = "On each other computer, run ctdev backup --all, choose Emergency Kit and password, then\n" +
	"  Paste an updated Emergency Kit, and paste this kit. If that computer can't read the destination\n" +
	"  any more, --all offers the paste straight away. No master password is needed there."

// unlockAccount asks for the master password and checks it by opening the
// destination with the account key, allowing three tries.
func unlockAccount(ctx context.Context, d *backup.Destination) (*backup.Account, error) {
	for range 3 {
		master, err := promptSecretRequiredCtx(ctx, "Master password", "")
		if err != nil {
			return nil, err
		}
		fmt.Println("  Checking the master password…")
		a, err := d.Unlock(ctx, master)
		if errors.Is(err, backup.ErrWrongPassword) {
			fmt.Println("  " + styles.Warning.Render("That master password doesn't open "+d.Name+"."))
			fmt.Println(styles.Dimmed.Render("  If its Secret Key was regenerated on another computer, paste the new kit first."))
			continue
		}
		return a, err
	}
	return nil, errors.New("three wrong tries; nothing was changed")
}

// confirm runs a [y/N] confirmation screen.
func confirm(c *backupscreen.Confirm) (bool, error) {
	if err := runScreen(c); err != nil {
		return false, err
	}
	return c.Confirmed(), nil
}

// manageComputers is Computers under --all: the list, then a computer's
// actions, until Back.
func manageComputers(ctx context.Context, d *backup.Destination) error {
	for {
		cs, err := d.Computers(ctx)
		if err != nil {
			return err
		}
		fmt.Println()
		list := backupscreen.NewComputers(d.Name, cs)
		if err := runScreen(list); err != nil {
			return err
		}
		c, ok := list.Chosen()
		if !ok {
			return nil
		}
		actions := backupscreen.NewComputerActions(c)
		if err := runScreen(actions); err != nil {
			return err
		}
		switch actions.Chosen() {
		case backupscreen.RemoveComputer:
			err = removeComputer(ctx, d, c)
		case backupscreen.LostComputer:
			err = lostComputer(ctx, d, c)
		}
		if err != nil {
			return err
		}
	}
}

func removeComputer(ctx context.Context, d *backup.Destination, c backup.Computer) error {
	fmt.Println(styles.Dimmed.Render("  If " + c.Hostname + " was lost or stolen, choose I lost this computer instead: it also replaces"))
	fmt.Println(styles.Dimmed.Render("  the Secret Key and the backend credentials, which a removed key leaves working."))
	ok, err := confirm(backupscreen.ConfirmRemoveComputer(c.Hostname, d.Name))
	if err != nil || !ok {
		return err
	}
	a, err := unlockAccount(ctx, d)
	if err != nil {
		return err
	}
	if err := a.RemoveComputer(ctx, c); err != nil {
		return err
	}
	fmt.Println("  " + styles.Success.Render("✓ Removed "+c.Hostname+"'s key: it can no longer open "+d.Name+"."))
	return nil
}

// lostComputer is "I lost this computer": remove its key, regenerate the
// Secret Key, take new backend credentials and revoke the old ones at the
// provider, then show the new kit to paste on every other computer.
// restic gives every key full power, so the lost computer could act on the
// repository with its key until removed, and with the old Secret Key and
// credentials on it, open it again from its kit until those are replaced.
func lostComputer(ctx context.Context, d *backup.Destination, c backup.Computer) error {
	ok, err := confirm(backupscreen.ConfirmLostComputer(c.Hostname, d.Name))
	if err != nil || !ok {
		return err
	}
	a, err := unlockAccount(ctx, d)
	if err != nil {
		return err
	}
	if err := a.RemoveComputer(ctx, c); err != nil {
		return err
	}
	fmt.Println("  " + styles.Success.Render("✓ 1. Removed "+c.Hostname+"'s key."))

	// A kit with an error means the new Secret Key is in force and only the
	// old account key's removal failed; the flow goes on.
	if kit, err := a.RegenerateSecretKey(ctx); kit == "" {
		return err
	} else if err != nil {
		fmt.Println("  " + styles.Warning.Render(err.Error()))
	}
	fmt.Println("  " + styles.Success.Render("✓ 2. Made a new Secret Key and saved it on this computer."))

	if err := replaceCredentials(ctx, a, d); err != nil {
		return err
	}
	fmt.Println("  " + styles.Success.Render("✓ 3. This computer opens "+d.Name+" with the new credentials."))

	if err := showKit(ctx, "New Emergency Kit", a.Kit(), ""); err != nil {
		return err
	}
	fmt.Println("\n  " + pasteOnOtherComputers)
	return nil
}

// replaceCredentials is step 3 of the lost-computer flow: new backend
// credentials, checked by opening the repository with them, then the old
// ones revoked at the provider. SFTP signs in with SSH keys, so there the
// lost computer's SSH key comes off the server first, and the check comes
// after, since that step can change this computer's own SSH key.
func replaceCredentials(ctx context.Context, a *backup.Account, d *backup.Destination) error {
	steps := providerSteps(d.Address)
	if len(steps.fields) == 0 {
		if steps.revoke != nil {
			fmt.Println("\n  Revoke the lost computer's access to the server:")
			printSteps(steps.revoke)
			if err := waitRevoked(ctx); err != nil {
				return err
			}
		}
		fmt.Println(styles.Dimmed.Render("  " + steps.note))
		if err := a.SetCredentials(ctx, nil); err != nil {
			return fmt.Errorf("this computer can't open %s: %w", d.Name, err)
		}
		return nil
	}

	fmt.Println("\n  Make new credentials for " + d.Name + ":")
	printSteps(steps.create)
	for {
		creds := map[string]string{}
		for _, f := range steps.fields {
			v, err := promptSecretRequiredCtx(ctx, f.label, "")
			if err != nil {
				return err
			}
			creds[f.name] = v
		}
		fmt.Println("  Checking them against the destination…")
		err := a.SetCredentials(ctx, creds)
		if err == nil {
			break
		}
		fmt.Println("  " + styles.Warning.Render("Those didn't open "+d.Name+" ("+err.Error()+"); nothing was saved. Enter them again."))
	}
	fmt.Println("\n  Now revoke the old credentials, which the lost computer still has:")
	printSteps(steps.revoke)
	if steps.note != "" {
		fmt.Println(styles.Dimmed.Render("  " + steps.note))
	}
	return waitRevoked(ctx)
}

func waitRevoked(ctx context.Context) error {
	fmt.Printf("\n  %s ", styles.Dimmed.Render("Press Enter once that's done."))
	if _, ok := readLineCtx(ctx); !ok {
		return errPromptCancelled
	}
	return nil
}

func printSteps(steps []string) {
	for i, s := range steps {
		fmt.Printf("    %d. %s\n", i+1, s)
	}
}

type credentialField struct{ name, label string }

// credentialSteps is how to replace a backend's credentials at its provider:
// the steps to make new ones, the fields ctdev asks for, and the steps to
// revoke the old ones. They follow each provider's documentation as of
// 2026-10-22: Backblaze's "Create and Manage App Keys", AWS IAM's "Manage
// access keys for IAM users", and OpenSSH's sshd(8) and ssh-keygen(1).
type credentialSteps struct {
	create []string
	fields []credentialField
	revoke []string
	note   string
}

func providerSteps(address string) credentialSteps {
	switch backup.BackendLabel(address) {
	case "Backblaze B2":
		return credentialSteps{
			create: []string{
				"Sign in to backblaze.com and open B2 Cloud Storage > Application Keys.",
				"Click Add a New Application Key.",
				"Name it, choose this destination's bucket under Allow Access to Bucket(s), and choose Read and Write.",
				"Click Create New Key.",
				"Copy the keyID and the applicationKey: the applicationKey is shown only once.",
			},
			fields: []credentialField{{"B2_ACCOUNT_ID", "New B2 keyID"}, {"B2_ACCOUNT_KEY", "New B2 applicationKey"}},
			revoke: []string{
				"In B2 Cloud Storage > Application Keys, find the old key under Your Application Keys by its keyID.",
				"Click Delete Key on it.",
			},
		}
	case "S3":
		if !strings.Contains(address, "amazonaws.com") {
			return credentialSteps{
				create: []string{
					"Sign in to your S3-compatible provider's console.",
					"Make a new access key that can read and write this destination's bucket.",
					"Copy its access key ID and secret access key.",
				},
				fields: []credentialField{{"AWS_ACCESS_KEY_ID", "New access key ID"}, {"AWS_SECRET_ACCESS_KEY", "New secret access key"}},
				revoke: []string{"In the same console, delete the old access key."},
				note:   "ctdev has no steps for this provider's console; its documentation has the exact clicks.",
			}
		}
		return credentialSteps{
			create: []string{
				"Open the IAM console at console.aws.amazon.com/iam and choose Users.",
				"Choose the user this destination's access key belongs to.",
				"On the Security credentials tab, under Access keys, choose Create access key. If it can't be chosen, the user already has two keys: delete the old one first (the steps below), then come back.",
				"Choose Other, then Next.",
				"Choose Create access key.",
				"Choose Show and copy the access key ID and the secret access key: this is the only time the secret is shown.",
			},
			fields: []credentialField{{"AWS_ACCESS_KEY_ID", "New access key ID"}, {"AWS_SECRET_ACCESS_KEY", "New secret access key"}},
			revoke: []string{
				"On the same Security credentials tab, under Access keys, find the old key by its access key ID.",
				"Choose Actions, then Deactivate, and Deactivate again to confirm.",
				"Choose Actions, then Delete, enter the access key ID, and choose Delete.",
			},
		}
	case "SFTP":
		return credentialSteps{
			revoke: []string{
				"Sign in to the server as the account restic uses, and open ~/.ssh/authorized_keys.",
				"Delete the line with the lost computer's public key. The comment at the end of each line usually names the computer; ssh-keygen -l -f <file> prints a key's fingerprint to match against.",
				"If your computers shared one SSH key, make a new one on each that's left (ssh-keygen -t ed25519), add it with ssh-copy-id <user@host>, and delete the shared key's line too.",
			},
			note: "ctdev then checks this computer still opens the destination over SFTP.",
		}
	}
	return credentialSteps{note: "A local path or USB drive has no credentials to revoke."}
}

// showKit shows a kit, and a new Recovery Code when there is one, and waits
// while they're saved.
func showKit(ctx context.Context, title, kit, code string) error {
	fmt.Println()
	fmt.Println(styles.Title.Render("  "+title) + styles.Dimmed.Render("  save it in your password manager or print it"))
	fmt.Println()
	for _, line := range strings.Split(strings.TrimRight(kit, "\n"), "\n") {
		fmt.Println("    " + line)
	}
	if code != "" {
		fmt.Println()
		fmt.Println(styles.Title.Render("  New Recovery Code") + styles.Dimmed.Render("  keep it apart from the kit; ctdev shows it once"))
		fmt.Println()
		fmt.Println("    " + styles.Value.Render(code))
	}
	fmt.Printf("\n  %s ", styles.Dimmed.Render("Press Enter once it's saved."))
	if _, ok := readLineCtx(ctx); !ok {
		return errPromptCancelled
	}
	return nil
}

// kitAndPassword is Emergency Kit and password under --all, until Back. It
// returns the destination as it now is: pasting a kit replaces it.
func kitAndPassword(ctx context.Context, m *backup.Machine, d *backup.Destination) (*backup.Destination, error) {
	for {
		fmt.Println()
		menu := backupscreen.NewKitMenu(d.Name)
		if err := runScreen(menu); err != nil {
			return d, err
		}
		var err error
		switch menu.Chosen() {
		case backupscreen.ShowKit:
			var a *backup.Account
			if a, err = unlockAccount(ctx, d); err == nil {
				err = showKit(ctx, "Emergency Kit", a.Kit(), "")
			}
		case backupscreen.ChangePassword:
			err = changeMasterPassword(ctx, d)
		case backupscreen.RegenerateKey:
			err = regenerateSecretKey(ctx, d)
		case backupscreen.UseRecoveryCode:
			_, err = recoveryReset(ctx, d.Name, d.Recover)
		case backupscreen.PasteKit:
			var nd *backup.Destination
			if nd, err = pasteUpdatedKit(ctx, m); err == nil && nd.Name == d.Name {
				d = nd
			}
		default:
			return d, nil
		}
		if err != nil {
			return d, err
		}
	}
}

func changeMasterPassword(ctx context.Context, d *backup.Destination) error {
	ok, err := confirm(backupscreen.ConfirmChangePassword(d.Name))
	if err != nil || !ok {
		return err
	}
	fmt.Println(styles.Dimmed.Render("  First the current master password."))
	a, err := unlockAccount(ctx, d)
	if err != nil {
		return err
	}
	fmt.Println(styles.Dimmed.Render("  Now the new one."))
	master, err := promptNewMasterPassword(ctx)
	if err != nil {
		return err
	}
	fmt.Println("  Changing it (this takes a few seconds)…")
	if err := a.ChangeMasterPassword(ctx, master); err != nil {
		return err
	}
	fmt.Println("  " + styles.Success.Render("✓ The master password is changed. The Emergency Kit and every computer's key keep working."))
	return nil
}

func regenerateSecretKey(ctx context.Context, d *backup.Destination) error {
	ok, err := confirm(backupscreen.ConfirmRegenerate(d.Name))
	if err != nil || !ok {
		return err
	}
	a, err := unlockAccount(ctx, d)
	if err != nil {
		return err
	}
	fmt.Println("  Making a new Secret Key (this takes a few seconds)…")
	kit, err := a.RegenerateSecretKey(ctx)
	if kit == "" {
		return err
	}
	if err != nil {
		fmt.Println("  " + styles.Warning.Render(err.Error()))
	}
	if err := showKit(ctx, "New Emergency Kit", kit, ""); err != nil {
		return err
	}
	fmt.Println("\n  " + pasteOnOtherComputers)
	return nil
}

// recoveryReset is Use a Recovery Code: the code, a new master password and
// whether to replace the code, then the new kit (and code) shown once.
// recover does the reset, on this computer's destination or a pasted kit.
func recoveryReset(ctx context.Context, name string, recover func(context.Context, string, string, bool) (backup.Recovery, error)) (*backup.Destination, error) {
	fmt.Println()
	fmt.Println(styles.Dimmed.Render("  The Recovery Code sets a new master password and a new Secret Key for " + name + "."))
	for range 3 {
		code, err := promptSecretRequiredCtx(ctx, "Recovery Code", "")
		if err != nil {
			return nil, err
		}
		fmt.Println(styles.Dimmed.Render("  Choose the new master password."))
		master, err := promptNewMasterPassword(ctx)
		if err != nil {
			return nil, err
		}
		newCode, err := confirm(backupscreen.ConfirmNewRecoveryCode())
		if err != nil {
			return nil, err
		}
		fmt.Println("  Resetting (this takes a few seconds)…")
		r, err := recover(ctx, code, master, newCode)
		if errors.Is(err, backup.ErrWrongPassword) {
			fmt.Println("  " + styles.Warning.Render("That Recovery Code doesn't open "+name+". Nothing was changed."))
			continue
		}
		if r.Kit == "" {
			return nil, err
		}
		if err != nil {
			fmt.Println("  " + styles.Warning.Render(err.Error()))
		}
		if err := showKit(ctx, "New Emergency Kit", r.Kit, r.RecoveryCode); err != nil {
			return nil, err
		}
		fmt.Println("\n  " + pasteOnOtherComputers)
		return r.Destination, nil
	}
	return nil, errors.New("three wrong tries; nothing was changed")
}

// readKit reads an Emergency Kit pasted at the prompt, asking again until it
// parses.
func readKit(ctx context.Context) (string, error) {
	fmt.Println()
	fmt.Println("  Paste the Emergency Kit, then press Enter on an empty line:")
	var lines []string
	for {
		line, ok := readLineCtx(ctx)
		if !ok {
			return "", errPromptCancelled
		}
		if line != "" {
			lines = append(lines, line)
			continue
		}
		if len(lines) == 0 {
			continue
		}
		kit := strings.Join(lines, "\n")
		if _, err := backup.ParseKit(kit); err != nil {
			fmt.Println("  " + styles.Warning.Render(err.Error()+": paste the whole kit again."))
			lines = nil
			continue
		}
		return kit, nil
	}
}

// pasteUpdatedKit is Paste an updated Emergency Kit: no master password,
// since this computer's own key opens the repository, and backup checks it
// does with the kit's details before saving them.
func pasteUpdatedKit(ctx context.Context, m *backup.Machine) (*backup.Destination, error) {
	kit, err := readKit(ctx)
	if err != nil {
		return nil, err
	}
	return updateFromKit(ctx, m, kit)
}

func updateFromKit(ctx context.Context, m *backup.Machine, kit string) (*backup.Destination, error) {
	fmt.Println("  Checking this computer's key against the destination with the kit…")
	d, err := m.UpdateFromKit(ctx, kit)
	if errors.Is(err, backup.ErrWrongPassword) {
		return nil, errors.New("this computer's key doesn't open the destination the kit describes, so nothing was saved: " +
			"its key may have been removed from another computer")
	}
	if err != nil {
		return nil, err
	}
	fmt.Println("  " + styles.Success.Render("✓ Saved the kit's credentials and Secret Key for "+d.Name+" on this computer."))
	return d, nil
}

// unreadable is --all's offer when the destination can't be read, as after
// another computer's lost-computer flow revoked the credentials this one
// has: paste the new kit, or quit with the error.
func unreadable(ctx context.Context, m *backup.Machine, d *backup.Destination, readErr error) (*backup.Destination, error) {
	if errors.Is(readErr, backup.ErrWrongPassword) {
		return nil, fmt.Errorf("this computer's key no longer opens %s: it may have been removed from another computer", d.Name)
	}
	fmt.Println("\n  " + styles.Error.Render("Can't read "+d.Name+": "+readErr.Error()))
	fmt.Println("  If its credentials were replaced on another computer, paste the new Emergency Kit.")
	fmt.Println("    1) Paste an updated Emergency Kit")
	fmt.Println("    2) Quit")
	fmt.Printf("  %s ", styles.Dimmed.Render("Choice [1]:"))
	choice, err := promptChoiceCtx(ctx, 1)
	if err != nil {
		return nil, err
	}
	if choice != 1 {
		return nil, readErr
	}
	return pasteUpdatedKit(ctx, m)
}

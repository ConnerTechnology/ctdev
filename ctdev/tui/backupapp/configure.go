package backupapp

import (
	"context"
	"errors"

	"github.com/ConnerTechnology/ctdev/ctdev/backup"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/styles"
)

// configureFolder is Configure: where backups go, the backup
// history for a new folder, how often and what to keep, each step opening
// on the folder's current values. esc steps back; nothing is saved until
// the last step, except a destination, created or connected in its own step.
func configureFolder(ctx context.Context, ui UI, d Deps, s backup.Status) error {
	if err := actionable(s); err != nil {
		return err
	}
	// From a subfolder, Target is the backed-up folder above it (CTD-87), so
	// Configure never sets up a folder inside one, which the package refuses.
	m := d.Machine
	where := backup.DisplayPath(m.Home, s.Target)
	contained, err := backup.CheckConfigurable(s.Target)
	var inside *backup.InsideError
	if errors.As(err, &inside) {
		ui.Say(Warning, "Already backed up as part of "+backup.DisplayPath(m.Home, inside.Root)+".")
		return ErrBack
	}
	if err != nil {
		return err
	}
	if len(contained) > 0 {
		body := "  " + styles.Warning.Render("These backed-up folders are inside "+where+", so they'll be in both folders' backups:") + "\n"
		for _, c := range contained {
			body += "\n    " + styles.Value.Render(backup.DisplayPath(m.Home, c))
		}
		ok, err := ui.Confirm(ctx, Confirm{
			Page:        Page{Title: where, Subtitle: styles.Dimmed.Render("Configure"), Body: body},
			Question:    "Configure " + where + " anyway?",
			Consequence: styles.Dimmed.Render("Each of them keeps its own backups too."),
			Yes:         "Configure anyway",
			Help:        "enter configure · esc back to the dashboard",
		})
		if err != nil {
			return err
		}
		if !ok {
			return ErrBack
		}
	}
	return newWizard(d, s).run(ctx, ui)
}

// focusAfterConfigure is the dashboard's focus once Configure has saved:
// Configure again while the folder isn't scheduled, Back up now for a folder
// that wasn't backed up before, else Quit.
func focusAfterConfigure(before, after backup.Status) string {
	switch {
	case after.Config == nil || !after.Scheduled || !after.SchedulerInstalled:
		return "c"
	case before.Config == nil:
		return "b"
	}
	return "q"
}

// createDestinationForOnce sets up this computer's first destination for a
// one-off backup, and returns it.
func createDestinationForOnce(ctx context.Context, ui UI, d Deps, s backup.Status) (*backup.Destination, error) {
	page := Page{Title: backup.DisplayPath(d.Machine.Home, s.Target),
		Subtitle: styles.Dimmed.Render("Back up once · ") + styles.Value.Render("New destination")}
	return newDestination(ctx, ui, d, func(string) Page { return page }, "",
		func(address string) (string, error) { return d.Machine.CheckNewAddress(address, s.Target) })
}

// createDestination is the new-destination step as --all shows
// it: name, backend, credentials, master password, creating it, and the kit
// shown once. check refuses an address (CheckNewAddress, or
// CheckNewAddressFromAll under --all).
func createDestination(ctx context.Context, ui UI, d Deps, preferred string, check func(string) (string, error)) (*backup.Destination, error) {
	return newDestination(ctx, ui, d, func(step string) Page {
		return Page{Title: "Backup destination › Create", Subtitle: styles.Dimmed.Render("Create a new destination · ") + styles.Value.Render(step)}
	}, preferred, check)
}

// connectDestination connects this computer with an Emergency Kit, as --all
// shows it.
func connectDestination(ctx context.Context, ui UI, d Deps) (*backup.Destination, error) {
	return connectWith(ctx, ui, d, func(step string) Page {
		return Page{Title: "Backup destination › Connect", Subtitle: styles.Dimmed.Render("Connect to an existing destination · ") + styles.Value.Render(step)}
	})
}

// recoveryReset sets a new master password and Secret Key with a Recovery
// Code, and shows the new kit. recover is
// Destination.Recover or a RecoverFromKit closure.
func recoveryReset(ctx context.Context, ui UI, d Deps, name string, recover func(ctx context.Context, code, master string, newCode bool) (backup.Recovery, error)) error {
	_, err := recoverWith(ctx, ui, func(step string) Page {
		return Page{Title: name + " › Emergency Kit and password › Use a Recovery Code",
			Subtitle: styles.Dimmed.Render("Use a Recovery Code · ") + styles.Value.Render(step)}
	}, name, name, recover)
	return err
}

// pasteUpdatedKit reads an Emergency Kit and updates this computer from it,
// and returns the destination it updated. It needs no
// master password: this computer's own key opens the repository, and the
// backup package checks it does with the kit's details before saving them.
// Its screens keep the page they open on.
func pasteUpdatedKit(ctx context.Context, ui UI, d Deps) (*backup.Destination, error) {
	refused := ""
	for {
		v, err := ui.Form(ctx, Form{
			Intro: "  " + styles.Value.Render("Paste the Emergency Kit.") + "\n  " +
				styles.Dimmed.Render("ctdev reads it as you paste; nothing is shown. No master password is needed."),
			Fields: []Field{kitField(refused)},
			Submit: "Check", Back: "Cancel",
		})
		if err != nil {
			return nil, err
		}
		dest, why, err := updateFromKit(ctx, ui, d, Page{}, v["kit"], "nothing is saved until this computer's key opens it")
		if why != "" || errors.Is(err, ErrBack) {
			refused = why
			continue
		}
		return dest, err
	}
}

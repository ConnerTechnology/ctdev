package backupapp

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"

	"github.com/ConnerTechnology/ctdev/ctdev/backup"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/styles"
)

// pageFor is the page a destination step shows, given the step's name.
// Configure's pages name their wizard section instead.
type pageFor func(step string) Page

// stores are where a new destination can keep its backups.
var stores = []Option{
	{Value: "b2", Label: "Backblaze B2"},
	{Value: "s3", Label: "Amazon S3 or S3-compatible"},
	{Value: "sftp", Label: "SFTP (another server over SSH)"},
	{Value: "local", Label: "Local path or USB drive"},
}

// placeFields are W5's fields: the name, the store, and each store's own
// fields, shown while it's chosen. Credentials are keyed by their
// environment variable. v holds what was typed before; errs, a refusal to
// show under a field.
func placeFields(v Values, errs map[string]string) []Field {
	host, _ := os.Hostname()
	field := func(store, key, label string, kind FieldKind, def string, required bool) Field {
		value, typed := v[key]
		if !typed {
			value = def
		}
		return Field{Key: key, Label: label, Kind: kind, Value: value, Required: required, Error: errs[key],
			Show: func(x Values) bool { return x["store"] == store }}
	}
	name := orDefault(v["name"], "backups")
	sftpHost := field("sftp", "sftp.host", "USER@HOST", Text, "", true)
	sftpHost.Note = "restic signs in with your own SSH keys and config."
	return []Field{
		{Key: "name", Label: "NAME", Value: name, Required: true, Error: errs["name"]},
		{Key: "store", Label: "STORE ON", Kind: Choice, Value: orDefault(v["store"], "b2"), Options: stores},
		field("b2", "b2.bucket", "BUCKET", Text, "", true),
		field("b2", "b2.path", "PATH IN BUCKET", Text, "ctdev", false),
		// Key IDs echo: they are identifiers, listed in the clear by the
		// provider and printed in the kit, and seeing one catches a bad paste.
		field("b2", "B2_ACCOUNT_ID", "KEY ID", Text, "", true),
		field("b2", "B2_ACCOUNT_KEY", "APPLICATION KEY", Secret, "", true),
		field("s3", "s3.bucket", "BUCKET", Text, "", true),
		field("s3", "s3.endpoint", "ENDPOINT", Text, "s3.amazonaws.com", true),
		field("s3", "s3.path", "PATH IN BUCKET", Text, "ctdev", false),
		field("s3", "AWS_ACCESS_KEY_ID", "ACCESS KEY ID", Text, "", true),
		field("s3", "AWS_SECRET_ACCESS_KEY", "SECRET KEY", Secret, "", true),
		sftpHost,
		field("sftp", "sftp.path", "PATH", Text, "/srv/backups/"+orDefault(host, "ctdev"), true),
		field("local", "local.path", "PATH", Text, "", true),
	}
}

// place is the restic repository and credentials W5's answers describe, and
// the field a refused address is shown under.
func place(v Values) (address string, creds map[string]string, at string) {
	creds = map[string]string{}
	switch v["store"] {
	case "b2":
		creds["B2_ACCOUNT_ID"], creds["B2_ACCOUNT_KEY"] = v["B2_ACCOUNT_ID"], v["B2_ACCOUNT_KEY"]
		return backup.B2Address(v["b2.bucket"], orDefault(v["b2.path"], "ctdev")), creds, "b2.bucket"
	case "s3":
		creds["AWS_ACCESS_KEY_ID"], creds["AWS_SECRET_ACCESS_KEY"] = v["AWS_ACCESS_KEY_ID"], v["AWS_SECRET_ACCESS_KEY"]
		return backup.S3Address(v["s3.endpoint"], v["s3.bucket"], orDefault(v["s3.path"], "ctdev")), creds, "s3.bucket"
	case "sftp":
		return backup.SFTPAddress(v["sftp.host"], v["sftp.path"]), creds, "sftp.path"
	}
	return v["local.path"], creds, "local.path"
}

// newDestination creates a destination (W5 to W10) on page: where it goes,
// checked by check before the master password is asked, so no password is
// asked for a place that can't work; then the master password, creating
// it, and the kit and Recovery Code shown once. esc on the first step
// returns ErrBack.
func newDestination(ctx context.Context, ui UI, d Deps, page pageFor, preferred string, check func(string) (string, error)) (*backup.Destination, error) {
	m := d.Machine
	if m.Opts.DryRun {
		return nil, backup.ErrDryRun
	}
	v := Values{"name": orDefault(preferred, "backups")}
	refused := map[string]string{}
	for {
		got, err := ui.Form(ctx, Form{Page: page("Where backups go"), Fields: placeFields(v, refused)})
		if err != nil {
			return nil, err
		}
		maps.Copy(v, got)
		clear(refused)
		address, creds, at := place(got)
		if address, err = check(address); err != nil {
			refused[at] = capitalize(err.Error()) + ": choose another place."
			continue
		}
		dest, err := createWith(ctx, ui, d, page, backup.NewDestination{Name: got["name"], Address: address, Credentials: creds})
		if errors.Is(err, ErrBack) {
			continue
		}
		return dest, err
	}
}

// createWith is W8 to W10: the master password, creating the destination,
// and its kit. esc on the password returns ErrBack.
func createWith(ctx context.Context, ui UI, d Deps, page pageFor, nd backup.NewDestination) (*backup.Destination, error) {
	v := Values{}
	var mismatch, failed string
	for {
		got, err := ui.Form(ctx, Form{Page: page("Master password"),
			Intro: "  " + styles.Dimmed.Render("The master password and a Secret Key ctdev generates together unlock the backups.") + "\n  " +
				styles.Dimmed.Render("ctdev never stores the master password: choose one you'll remember."),
			Fields: newPasswordFields(v, mismatch), Error: failed, Submit: "Create"})
		if err != nil {
			return nil, err
		}
		v, mismatch, failed = got, "", ""
		if got["master"] != got["again"] {
			mismatch, v["again"] = "Those don't match; try again.", ""
			continue
		}

		creating := page("Creating")
		creating.Body = Row("NAME", styles.Value.Render(nd.Name)) + "\n" +
			Row("STORE ON", styles.Value.Render(backup.BackendLabel(nd.Address))+"  "+styles.Dimmed.Render(nd.Address))
		var setup backup.Setup
		err = ui.Run(ctx, Running{Page: creating, Label: "Creating the destination…",
			Detail: "restic sets up the repository there; this takes a few seconds"},
			func(ctx context.Context, _ func(Progress)) error {
				var err error
				setup, err = d.Machine.CreateDestination(ctx, nd, got["master"])
				return err
			})
		switch {
		case errors.Is(err, ErrBack):
			continue
		case errors.Is(err, ErrCancelled):
			return nil, err
		case err != nil:
			failed = capitalize(err.Error())
			continue
		}
		err = ui.ShowKit(ctx, KitScreen{Page: page("Emergency Kit"), Kit: setup.Kit, RecoveryCode: setup.RecoveryCode, Lines: []Line{
			{Warning, "ctdev shows these once. Lose the kit and the master password, with no Recovery Code"},
			{Warning, "and no computer still set up, and the backups can't be decrypted by anyone."},
		}})
		// The destination exists now: leaving the kit unsaved doesn't undo it.
		if err != nil && !errors.Is(err, ErrBack) {
			return nil, err
		}
		return setup.Destination, nil
	}
}

// newPasswordFields are a new master password, typed twice since it's
// masked. v holds what was typed before; mismatch is the second's error.
func newPasswordFields(v Values, mismatch string) []Field {
	return []Field{
		{Key: "master", Label: "MASTER PASSWORD", Kind: Secret, Required: true, Value: v["master"], Validate: masterPasswordRule},
		{Key: "again", Label: "TYPE IT AGAIN", Kind: Secret, Required: true, Value: v["again"], Error: mismatch},
	}
}

func kitField(refused string) Field {
	return Field{Key: "kit", Label: "EMERGENCY KIT", Kind: KitPaste, Required: true, Error: refused}
}

// kitLine is a pasted kit as a form shows it, without its secrets.
func kitLine(kit string) string {
	k, _ := backup.ParseKit(kit)
	return "  " + styles.Dimmed.Render(fmt.Sprintf("%-*s", labelWidth, "EMERGENCY KIT")) + "   " + styles.Success.Render("✓") + " " +
		styles.Value.Render(fmt.Sprintf("Pasted an Emergency Kit for %s (%s)", k.Destination, backup.BackendLabel(k.Address)))
}

// connectWith connects this computer with an Emergency Kit (X1 to X11) on
// page. A kit for a destination it's already on updates it instead, with no
// master password. esc on the paste returns ErrBack.
func connectWith(ctx context.Context, ui UI, d Deps, page pageFor) (*backup.Destination, error) {
	m := d.Machine
	if m.Opts.DryRun {
		return nil, backup.ErrDryRun
	}
	refused := ""
	for {
		v, err := ui.Form(ctx, Form{Page: page("Emergency Kit"), Intro: "  " + styles.Dimmed.Render("Paste the Emergency Kit. It isn't shown on screen."),
			Fields: []Field{kitField(refused)}})
		if err != nil {
			return nil, err
		}
		kit := v["kit"]
		refused = ""
		if m.KitIsSetUp(kit) {
			// An updated kit, after a new Secret Key or a lost computer
			// elsewhere: this computer's own key vouches for it.
			dest, why, err := updateFromKit(ctx, ui, d, page("Checking"), kit,
				"This computer is already set up on that destination; updating it from the kit.")
			if why != "" || errors.Is(err, ErrBack) {
				refused = why
				continue
			}
			return dest, err
		}
		dest, err := connectWithPassword(ctx, ui, d, page, kit)
		if errors.Is(err, ErrBack) {
			continue
		}
		return dest, err
	}
}

// connectWithPassword is X3 to X6 and X11: the master password, three tries
// of it, and the Recovery Code instead. esc returns ErrBack, to paste
// another kit.
func connectWithPassword(ctx context.Context, ui UI, d Deps, page pageFor, kit string) (*backup.Destination, error) {
	k, _ := backup.ParseKit(kit)
	failed := ""
	for tries := 0; ; {
		v, err := ui.Form(ctx, Form{Page: page("Master password"), Intro: kitLine(kit),
			Fields: []Field{{Key: "master", Label: "MASTER PASSWORD", Kind: Secret, Required: true}}, Error: failed, Submit: "Connect"})
		if err != nil {
			return nil, err
		}
		failed = ""
		var dest *backup.Destination
		err = ui.Run(ctx, Running{Page: page("Checking"), Label: "Checking the kit and password against the destination…",
			Detail: "restic opens the destination with them"},
			func(ctx context.Context, _ func(Progress)) error {
				var err error
				dest, err = d.Machine.ConnectDestination(ctx, kit, v["master"])
				return err
			})
		switch {
		case errors.Is(err, ErrBack):
			continue
		case errors.Is(err, backup.ErrWrongPassword):
			tries++
			dest, err := afterWrongPassword(ctx, ui, d, page, kit, k.Destination, tries)
			if errors.Is(err, errTryAgain) {
				continue
			}
			if err != nil {
				return nil, err
			}
			ui.Say(OK, "✓ This computer is set up on "+dest.Name+" with its own key.")
			return dest, nil
		case errors.Is(err, ErrCancelled):
			return nil, err
		case err != nil:
			failed = capitalize(err.Error())
			continue
		}
		ui.Say(OK, "✓ This computer is set up on "+dest.Name+" with its own key.")
		return dest, nil
	}
}

// errTryAgain is X6's "Try the master password again".
var errTryAgain = errors.New("try the master password again")

// afterWrongPassword is X6: try the master password again (errTryAgain),
// up to three tries in all, or reset with a Recovery Code.
func afterWrongPassword(ctx context.Context, ui UI, d Deps, page pageFor, kit, name string, tries int) (*backup.Destination, error) {
	for {
		i, err := ui.Pick(ctx, Picker{Page: page("Master password"),
			Intro: kitLine(kit) + "\n  " + styles.Dimmed.Render(fmt.Sprintf("%-*s", labelWidth, "MASTER PASSWORD")) + "   " + styles.Value.Render("••••••••••") +
				"\n    " + styles.Error.Render("That master password and Secret Key don't open the destination. Nothing was saved."),
			Items: []Item{{Cells: []string{"Try the master password again"}},
				{Cells: []string{"Use a Recovery Code (sets a new master password and Secret Key)"}}},
			Radio: true, Note: fmt.Sprintf("%d of 3 tries used.", tries), Enter: "Next", Help: "↑/↓ choose · enter next"})
		switch {
		case errors.Is(err, ErrBack), err == nil && i == 0:
			if tries >= 3 {
				return nil, errors.New("three wrong tries; nothing was saved")
			}
			return nil, errTryAgain
		case err != nil:
			return nil, err
		}
		dest, err := recoverWith(ctx, ui, page, "the destination", name, func(ctx context.Context, code, master string, newCode bool) (backup.Recovery, error) {
			return d.Machine.RecoverFromKit(ctx, kit, code, master, newCode)
		})
		if errors.Is(err, ErrBack) {
			continue
		}
		return dest, err
	}
}

// recoverWith is a Recovery Code reset on page (X7 to X10, P11 to P13): the
// code, a new master password, and whether to replace the code, then the
// new kit (and code) shown once. name is the destination as the text calls
// it; keysOn, its name. esc returns ErrBack.
func recoverWith(ctx context.Context, ui UI, page pageFor, name, keysOn string, recover func(ctx context.Context, code, master string, newCode bool) (backup.Recovery, error)) (*backup.Destination, error) {
	v := Values{"newcode": "no"}
	var wrong, mismatch, failed string
	for tries := 0; tries < 3; {
		fields := append([]Field{{Key: "code", Label: "RECOVERY CODE", Kind: Secret, Required: true, Value: v["code"], Error: wrong}},
			newPasswordFields(v, mismatch)...)
		fields = append(fields, Field{Key: "newcode", Label: "NEW CODE", Kind: Choice, Value: v["newcode"], Options: []Option{
			{Value: "no", Label: "No, keep the current one"}, {Value: "yes", Label: "Yes, the old one stops working"}}})
		got, err := ui.Form(ctx, Form{Page: page("Reset"),
			Intro: "  " + styles.Dimmed.Render("The Recovery Code sets a new master password and a new Secret Key for "+name+".") + "\n  " +
				styles.Dimmed.Render("Choose the new master password."),
			Fields: fields, Error: failed, Submit: "Reset"})
		if err != nil {
			return nil, err
		}
		v, wrong, mismatch, failed = got, "", "", ""
		if got["master"] != got["again"] {
			mismatch, v["again"] = "Those don't match; try again.", ""
			continue
		}
		var r backup.Recovery
		err = ui.Run(ctx, Running{Page: page("Resetting"), Label: "Resetting…",
			Detail: "a new master password and Secret Key for " + name + "; this takes a few seconds", ChangesKeysOn: keysOn},
			func(ctx context.Context, _ func(Progress)) error {
				var err error
				// Not cancelled when ctdev quits, so the reset isn't left
				// halfway, and what it made is said.
				r, err = recover(context.WithoutCancel(ctx), got["code"], got["master"], got["newcode"] == "yes")
				return err
			})
		switch {
		case errors.Is(err, backup.ErrWrongPassword):
			tries++
			wrong, v["code"] = "That Recovery Code doesn't open "+name+". Nothing was changed.", ""
			continue
		case r.Kit == "" && errors.Is(err, ErrCancelled):
			return nil, err
		case r.Kit == "" && err != nil:
			failed = capitalize(err.Error())
			continue
		}
		var lines []Line
		if err != nil {
			// Part of the reset failed; the kit is what is in force.
			lines = append(lines, Line{Warning, capitalize(err.Error())}, Line{})
		}
		lines = append(lines, paragraph(Info, pasteOnOtherComputers, frameWidth-4)...)
		err = ui.ShowKit(ctx, KitScreen{Page: page("New Emergency Kit"), Kit: r.Kit, RecoveryCode: r.RecoveryCode, Lines: lines, New: true})
		// The reset is done: leaving the kit unsaved doesn't undo it.
		if errors.Is(err, ErrCancelled) {
			ui.Say(Warning, newKitNotShown)
			if r.RecoveryCode != "" {
				// Show Emergency Kit never shows a Recovery Code.
				ui.Say(Warning, "A new Recovery Code was made too, and it can't be shown again: the old one no longer works.")
			}
		}
		if err != nil && !errors.Is(err, ErrBack) {
			return nil, err
		}
		ui.Say(Info, pasteOnOtherComputers)
		return r.Destination, nil
	}
	return nil, errors.New("three wrong tries; nothing was changed")
}

// updateFromKit updates a destination this computer is on from a pasted
// kit (X4, P15, P16) and says so. why is the refusal to show under the kit
// field, for another paste; ErrBack means checking was cancelled.
func updateFromKit(ctx context.Context, ui UI, d Deps, page Page, kit, detail string) (dest *backup.Destination, why string, err error) {
	var newSecretKey bool
	err = ui.Run(ctx, Running{Page: page, Label: "Checking this computer's key against the destination with the kit…", Detail: detail},
		func(ctx context.Context, _ func(Progress)) error {
			var err error
			dest, newSecretKey, err = d.Machine.UpdateFromKit(ctx, kit)
			return err
		})
	switch {
	case errors.Is(err, backup.ErrWrongPassword):
		return nil, "This computer's key doesn't open the destination the kit describes, so nothing was saved: " +
			"its key may have been removed from another computer.", nil
	case errors.Is(err, ErrBack), errors.Is(err, ErrCancelled):
		return nil, "", err
	case err != nil:
		return nil, capitalize(err.Error()) + ".", nil
	}
	ui.Say(OK, "✓ Saved the kit's credentials and Secret Key for "+dest.Name+" on this computer.")
	if newSecretKey {
		// This computer's key vouches for the address and credentials, but
		// nothing vouches for a Secret Key without the master password.
		ui.Say(Warning, "The kit's Secret Key is different from the one this computer had.")
		ui.Say(Info, "That's expected after Regenerate Secret Key or I lost this computer on another computer. "+
			"If neither happened, the kit didn't come from you: check it, and use Show Emergency Kit on a computer you trust.")
	}
	return dest, "", nil
}

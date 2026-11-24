package backupapp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ConnerTechnology/ctdev/ctdev/backup"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/styles"
)

// The --all actions on a destination's computers and secrets (CTD-124,
// ADR-0016). Each asks for the master password first and goes through
// backup.Account, which only a verified master password unlocks; a Recovery
// Code reset and pasting an updated kit are the exceptions.

// pasteOnOtherComputers is what to do with a new kit everywhere else. It
// names the menu path that takes it, and the offer --all makes when the
// destination can't be read with the old one.
const pasteOnOtherComputers = "On each other computer, run ctdev backup --all, choose Emergency Kit and password, then " +
	"Paste an updated Emergency Kit, and paste this kit. If that computer can't read the destination " +
	"any more, --all offers the paste straight away. No master password is needed there."

// errDeclined is an action whose confirmation was answered no: nothing
// happened, and nothing needs saying.
var errDeclined = errors.New("declined")

// endAction says how an action ended on the screen it returns to, and
// passes on the errors that leave that screen too.
func endAction(ui UI, err error) error {
	switch {
	case err == nil, errors.Is(err, errDeclined):
		return nil
	case errors.Is(err, ErrBack):
		ui.Say(Info, "Cancelled.")
		return nil
	case errors.Is(err, ErrCancelled), errors.Is(err, errToKitMenu):
		return err
	}
	sayError(ui, err)
	return nil
}

// confirmAction asks c, and turns no into errDeclined.
func confirmAction(ctx context.Context, ui UI, c Confirm) error {
	ok, err := ui.Confirm(ctx, c)
	switch {
	case err != nil:
		return err
	case !ok:
		return errDeclined
	}
	return nil
}

// subtitle is a step's: "Remove example-desktop · Master password".
func subtitle(action, step string) string {
	return styles.Dimmed.Render(action+" · ") + styles.Value.Render(step)
}

// unlockAccount asks for the master password on page and checks it by
// opening the destination with the account key, allowing three tries.
func unlockAccount(ctx context.Context, ui UI, dest *backup.Destination, page Page, intro string) (*backup.Account, error) {
	dim := styles.Dimmed.Render
	f := Form{Page: page, Intro: intro, Submit: "Unlock", Back: "Cancel",
		Fields: []Field{{Key: "master", Label: "MASTER PASSWORD", Kind: Secret, Required: true}}}
	for tries := 3; tries > 0; tries-- {
		v, err := ui.Form(ctx, f)
		if err != nil {
			return nil, err
		}
		var a *backup.Account
		err = ui.Run(ctx, Running{Label: "Checking the master password…", Detail: "opening " + dest.Name + " with the account key"},
			func(ctx context.Context, _ func(Progress)) error {
				var err error
				a, err = dest.Unlock(ctx, v["master"])
				return err
			})
		if !errors.Is(err, backup.ErrWrongPassword) {
			return a, err
		}
		left := "1 try left"
		if tries > 2 {
			left = fmt.Sprintf("%d tries left", tries-1)
		}
		f.Fields[0].Error = "That master password doesn't open " + dest.Name + "."
		f.Intro = strings.TrimLeft(intro+"\n", "\n") +
			"  " + dim("If its Secret Key was regenerated on another computer, paste the new kit first.") + "\n  " + dim(left)
	}
	return nil, errors.New("Three wrong tries; nothing was changed.")
}

// manageComputers is Computers under --all: the list, then a computer's
// actions, until Back.
func manageComputers(ctx context.Context, ui UI, d Deps, dest *backup.Destination) error {
	dim, val := styles.Dimmed.Render, styles.Value.Render
	page := Page{Title: crumb(dest.Name, "Computers"), Subtitle: dim("Computers with access to ") + val(dest.Name)}
	cursor := 0
	for {
		var cs []backup.Computer
		err := ui.Run(ctx, Running{Page: page, Label: "Reading the computers…", Detail: "restic key list on " + dest.Name},
			func(ctx context.Context, _ func(Progress)) error {
				var err error
				cs, err = dest.Computers(ctx)
				return err
			})
		if err != nil {
			return err
		}
		i, err := ui.Pick(ctx, computersPicker(page, cs, cursor))
		if errors.Is(err, ErrBack) {
			return nil
		}
		if err != nil {
			return err
		}
		cursor = i
		if err := computerActions(ctx, ui, dest, cs[i]); err != nil {
			return err
		}
	}
}

// computersPicker lists each computer with a key on the destination, by
// hostname and date added, this one marked. Computers leaves out the account
// and Recovery Code keys.
func computersPicker(page Page, cs []backup.Computer, cursor int) Picker {
	dim := styles.Dimmed.Render
	items := make([]Item, len(cs))
	for i, c := range cs {
		this := ""
		if c.ThisComputer {
			this = dim("this computer")
		}
		items[i] = Item{Cells: []string{c.Hostname, dim(addedOn(c)), this}}
	}
	return Picker{Page: page, Columns: []Column{{Title: "COMPUTER"}, {Title: "ADDED"}, {}}, Items: items, Cursor: cursor}
}

func addedOn(c backup.Computer) string { return c.Added.Local().Format("Jan 2, 2006") }

// computerActions is one computer's screen, until Back or an action that
// finishes: Remove this computer · I lost this computer. This computer's own
// key can't be removed from here, so for it both are dimmed.
func computerActions(ctx context.Context, ui UI, dest *backup.Destination, c backup.Computer) error {
	for {
		a, err := ui.Dashboard(ctx, computerDashboard(dest.Name, c))
		if errors.Is(err, ErrBack) {
			return nil
		}
		if err != nil {
			return err
		}
		switch a.Key {
		case "R":
			err = removeComputer(ctx, ui, dest, c)
		case "L":
			err = lostComputer(ctx, ui, dest, c)
		default:
			return nil
		}
		if err == nil {
			return nil
		}
		if err := endAction(ui, err); err != nil {
			return err
		}
	}
}

func computerDashboard(name string, c backup.Computer) Dashboard {
	dim, val := styles.Dimmed.Render, styles.Value.Render
	body := Row("COMPUTER", styles.Bold.Render(c.Hostname)) + "\n" + Row("ADDED", val(addedOn(c)))
	if c.ThisComputer {
		body += "\n\n  " + dim("This computer: remove it from another computer that's set up.")
	}
	return Dashboard{
		Page: Page{Title: crumb(name, "Computers", c.Hostname), Subtitle: dim("Computers with access to ") + val(name), Body: body},
		Actions: []Action{
			{Key: "R", Label: "Remove this computer", Dimmed: c.ThisComputer},
			{Key: "L", Label: "I lost this computer", Dimmed: c.ThisComputer},
			{Key: "esc", Label: "Back"},
		},
		Help: "←/→ choose · enter open · or press the letter",
	}
}

func removeComputer(ctx context.Context, ui UI, dest *backup.Destination, c backup.Computer) error {
	host, name := c.Hostname, dest.Name
	if err := confirmAction(ctx, ui, Confirm{
		Warning: "If " + host + " was lost or stolen, choose I lost this computer instead: it also replaces\n" +
			"  the Secret Key and the backend credentials, which a removed key leaves working.",
		Question:    "Remove " + host + "'s key?",
		Consequence: styles.Dimmed.Render(host + " can no longer open " + name + "."),
		Yes:         "Remove", Destructive: true,
	}); err != nil {
		return err
	}
	title := crumb(name, "Computers", host, "Remove")
	a, err := unlockAccount(ctx, ui, dest, Page{Title: title, Subtitle: subtitle("Remove "+host, "Master password")}, "")
	if err != nil {
		return err
	}
	err = ui.Run(ctx, Running{Page: Page{Title: title, Subtitle: subtitle("Remove "+host, "Removing")},
		Label: "Removing " + host + "'s key…", Detail: "restic key remove on " + name, ChangesKeysOn: name},
		func(ctx context.Context, _ func(Progress)) error { return a.RemoveComputer(ctx, c) })
	if err != nil {
		return err
	}
	ui.Say(OK, "✓ Removed "+host+"'s key: it can no longer open "+name+".")
	return nil
}

// lostSteps is the checklist of I lost this computer: how far it got, and
// what stopped it.
type lostSteps struct {
	host, name string
	done       int    // steps finished, 0 to 3
	failed     error  // why step done+1 failed
	warning    string // step 2's: the new Secret Key is in force, but the old account key's removal failed
}

// lines are the checklist: ✓ for each step done, ✗ for the one that failed,
// ○ for the rest.
func (l *lostSteps) lines() string {
	done := []string{
		"✓ 1. Removed " + l.host + "'s key.",
		"✓ 2. Made a new Secret Key and saved it on this computer.",
		"✓ 3. This computer opens " + l.name + " with the new credentials.",
	}
	todo := []string{"1. Remove " + l.host + "'s key", "2. Make a new Secret Key", "3. New backend credentials"}
	var lines []string
	for i := range 3 {
		switch {
		case i < l.done:
			lines = append(lines, "  "+styles.Success.Render(done[i]))
			if i == 1 && l.warning != "" {
				lines = append(lines, "    "+styles.Warning.Render(l.warning))
			}
		case i == l.done && l.failed != nil:
			lines = append(lines, "  "+styles.Error.Render("✗ "+todo[i]), "    "+styles.Error.Render(oneLine(l.failed)))
		default:
			lines = append(lines, "  "+styles.Dimmed.Render("○ "+todo[i]))
		}
	}
	return strings.Join(lines, "\n")
}

// page is the flow's at step: the checklist in its body when withList.
func (l *lostSteps) page(step string, withList bool) Page {
	p := Page{Title: crumb(l.name, "Computers", l.host, "I lost this computer"),
		Subtitle: subtitle(fmt.Sprintf("I lost %s · step %d of 3", l.host, min(l.done+1, 3)), step)}
	if withList {
		p.Body = l.lines()
	}
	return p
}

// lostComputer is "I lost this computer": remove its key, regenerate the
// Secret Key, take new backend credentials and revoke the old ones at the
// provider, then show the new kit to paste on every other computer.
// restic gives every key full power, so the lost computer could act on the
// repository with its key until removed, and with the old Secret Key and
// credentials on it, open it again from its kit until those are replaced.
func lostComputer(ctx context.Context, ui UI, dest *backup.Destination, c backup.Computer) (err error) {
	host, name := c.Hostname, dest.Name
	if err := confirmAction(ctx, ui, Confirm{
		Question:    "Remove " + host + "'s key from " + name + ", make a new Secret Key and take new backend credentials?",
		Consequence: styles.Dimmed.Render("You'll paste the new kit once on every other computer."),
		Yes:         "Start", Destructive: true,
	}); err != nil {
		return err
	}
	l := &lostSteps{host: host, name: name}
	a, err := unlockAccount(ctx, ui, dest, Page{Title: l.page("", false).Title, Subtitle: subtitle("I lost "+host, "Master password")}, "")
	if err != nil {
		return err
	}
	err = ui.Run(ctx, Running{Page: l.page("Remove the key", true), Label: "Removing " + host + "'s key…",
		Detail: "restic key remove on " + name, ChangesKeysOn: name},
		func(ctx context.Context, _ func(Progress)) error { return a.RemoveComputer(ctx, c) })
	if err != nil {
		return err
	}
	l.done = 1

	// From here each step has changed the destination, so a stop says what
	// was done and where to finish: the flow can't be started again for a
	// computer that's no longer listed.
	defer func() {
		if err != nil {
			err = l.stopped(ctx, ui, err)
		}
	}()

	// A kit with an error means the new Secret Key is in force and only the
	// old account key's removal failed; the flow goes on.
	err = ui.Run(ctx, Running{Page: l.page("New Secret Key", true), Label: "Making a new Secret Key…",
		Detail: "a new Secret Key and Emergency Kit, then the old account key removed", ChangesKeysOn: name},
		func(ctx context.Context, _ func(Progress)) error {
			kit, err := a.RegenerateSecretKey(ctx)
			if kit != "" && err != nil {
				l.warning = err.Error()
				return nil
			}
			return err
		})
	if err != nil {
		return err
	}
	l.done = 2

	pages := func(step string) Page { return l.page(step, false) }
	if err := replaceCredentials(ctx, ui, a, dest, pages, l.lines, "Stop"); err != nil {
		return err
	}
	l.done = 3

	if err := ui.ShowKit(ctx, KitScreen{Page: l.page("New Emergency Kit", true), Kit: a.Kit(), Lines: newKitLines()}); err != nil {
		return err
	}
	ui.Say(Info, pasteOnOtherComputers)
	return nil
}

// stopped says what the flow did before err stopped it, and which items
// under Emergency Kit and password finish it, then offers that menu. The
// lines are also printed after ctdev exits, so they outlive the screen.
func (l *lostSteps) stopped(ctx context.Context, ui UI, err error) error {
	if !errors.Is(err, ErrBack) && !errors.Is(err, ErrCancelled) {
		l.failed = err
	}
	var did, next string
	switch l.done {
	case 1:
		did = "removed " + l.host + "'s key. The Secret Key and the backend credentials are unchanged"
		next = "Regenerate Secret Key, then Replace backend credentials"
	case 2:
		did = "removed " + l.host + "'s key and made a new Secret Key, saved on this computer. The backend credentials are unchanged"
		next = "Replace backend credentials, which also shows the new kit"
	default:
		did = "removed " + l.host + "'s key, made a new Secret Key and saved the new backend credentials. The new kit wasn't shown"
		next = "Show Emergency Kit"
	}
	msg := []Line{
		{Warning, "I lost this computer stopped. So far it " + did + "."},
		{Plain, "To finish, run ctdev backup --all, choose Emergency Kit and password, then " + next + "."},
	}
	for _, line := range msg {
		ui.Say(line.Kind, line.Text)
	}
	if errors.Is(err, ErrCancelled) {
		return err
	}
	page := l.page("Stopped", true)
	page.Badge = Badge{Attention, "STOPPED"}
	a, err := ui.Dashboard(ctx, Dashboard{Page: page, Message: msg, Actions: []Action{
		{Key: "e", Label: "Emergency Kit and password"},
		{Key: "esc", Label: "Back to Computers"},
	}})
	switch {
	case errors.Is(err, ErrBack):
		return nil
	case err != nil:
		return err
	case a.Key == "e":
		return errToKitMenu
	}
	return nil
}

// newKitLines go under a new kit: what it replaces, and where to paste it.
func newKitLines() []Line {
	return append([]Line{{Warning, "The old Emergency Kit stops working."}}, paragraph(Info, pasteOnOtherComputers, frameWidth-4)...)
}

// paragraph is text as lines of kind, broken at spaces to fit width.
func paragraph(kind Kind, text string, width int) []Line {
	var lines []Line
	line := ""
	for _, word := range strings.Fields(text) {
		if line != "" && len(line)+1+len(word) > width {
			lines = append(lines, Line{kind, line})
			line = ""
		}
		if line != "" {
			line += " "
		}
		line += word
	}
	return append(lines, Line{kind, line})
}

// replaceCredentials is step 3 of the lost-computer flow: new backend
// credentials, checked by opening the repository with them, then the old
// ones revoked at the provider. SFTP signs in with SSH keys, so there the
// lost computer's SSH key comes off the server first, and the check comes
// after, since that step can change this computer's own SSH key. page is
// each screen's, by step; intro, when set, is the checklist above them.
func replaceCredentials(ctx context.Context, ui UI, a *backup.Account, dest *backup.Destination, page func(step string) Page, intro func() string, back string) error {
	name := dest.Name
	head := func(extra ...string) string {
		var parts []string
		if intro != nil {
			parts = append(parts, intro())
		}
		return strings.Join(append(parts, extra...), "\n\n")
	}
	check := func(step string, creds map[string]string, detail string) error {
		p := page(step)
		p.Body = head()
		return ui.Run(ctx, Running{Page: p, Label: "Checking them against the destination…", Detail: detail, ChangesKeysOn: name},
			func(ctx context.Context, _ func(Progress)) error { return a.SetCredentials(ctx, creds) })
	}

	steps := providerSteps(dest.Address)
	if len(steps.fields) == 0 {
		step, detail := "Check", steps.note
		if steps.revoke != nil {
			step, detail = "Revoke the lost computer's access", "opening "+name+" over SFTP with this computer's SSH key"
			if err := ui.Notice(ctx, Notice{Page: page(step), Intro: head(), Heading: "Revoke the lost computer's access to the server:",
				Steps: steps.revoke, Note: steps.note, Ack: "Done, check it", Back: back}); err != nil {
				return err
			}
		}
		err := check(step, nil, detail)
		if err != nil && !errors.Is(err, ErrCancelled) {
			return fmt.Errorf("this computer can't open %s: %w", name, err)
		}
		return err
	}

	create := styles.Header.Render("  Make new credentials for "+name+":") + "\n" + numbered(steps.create)
	if steps.note != "" {
		create += "\n\n  " + styles.Dimmed.Render(steps.note)
	}
	fields := make([]Field, len(steps.fields))
	for i, f := range steps.fields {
		fields[i] = Field{Key: f.name, Label: strings.ToUpper(f.label), Kind: f.kind, Required: true}
	}
	for {
		v, err := ui.Form(ctx, Form{Page: page("New credentials"), Intro: head(create), Fields: fields, Submit: "Check", Back: back})
		if err != nil {
			return err
		}
		creds := map[string]string{}
		for _, f := range steps.fields {
			creds[f.name] = v[f.name]
		}
		err = check("New credentials", creds, "opening "+name+" with them before saving anything")
		if err == nil {
			break
		}
		if errors.Is(err, ErrCancelled) {
			return err
		}
		fields[0].Error = "Those didn't open " + name + " (" + err.Error() + "); nothing was saved. Enter them again."
	}
	opened := "  " + styles.Success.Render("✓") + " " + styles.Value.Render("The new credentials open "+name+".")
	return ui.Notice(ctx, Notice{Page: page("Revoke the old credentials"), Intro: head(opened),
		Heading: "Now revoke the old credentials, which the lost computer still has:",
		Steps:   steps.revoke, Note: steps.note, Ack: "Done, they're revoked", Back: back})
}

// numbered is steps as the Notice shows them: numbered, and wrapped under
// their text.
func numbered(steps []string) string {
	var lines []string
	for i, s := range steps {
		num := fmt.Sprintf("%d. ", i+1)
		for j, l := range wrap(Line{Plain, s}, frameWidth-2-len(num), 2) {
			pre := "    " + styles.Dimmed.Render(num)
			if j > 0 {
				pre = strings.Repeat(" ", 4+len(num))
			}
			lines = append(lines, pre+strings.TrimPrefix(l, "  "))
		}
	}
	return strings.Join(lines, "\n")
}

type credentialField struct {
	name, label string
	kind        FieldKind // an identifier echoes; a secret is masked
}

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
			fields: []credentialField{{"B2_ACCOUNT_ID", "New B2 keyID", Text}, {"B2_ACCOUNT_KEY", "New B2 applicationKey", Secret}},
			revoke: []string{
				"In B2 Cloud Storage > Application Keys, find the old key under Your Application Keys by its keyID.",
				"Click Delete Key on it.",
			},
		}
	case "S3":
		fields := []credentialField{{"AWS_ACCESS_KEY_ID", "New access key ID", Text}, {"AWS_SECRET_ACCESS_KEY", "New secret access key", Secret}}
		if !strings.Contains(address, "amazonaws.com") {
			return credentialSteps{
				create: []string{
					"Sign in to your S3-compatible provider's console.",
					"Make a new access key that can read and write this destination's bucket.",
					"Copy its access key ID and secret access key.",
				},
				fields: fields,
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
			fields: fields,
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

// kitAction is an item of Emergency Kit and password.
type kitAction struct {
	label, detail string
	unlocks       bool // it asks for the master password
	run           func() error
}

// kitAndPassword is Emergency Kit and password under --all, until Back. It
// returns the destination as it now is: pasting a kit replaces it.
func kitAndPassword(ctx context.Context, ui UI, d Deps, dest *backup.Destination) (*backup.Destination, error) {
	actions := []kitAction{
		{"Show Emergency Kit", "the kit to save or print", true, func() error { return showKit(ctx, ui, dest) }},
		{"Change master password", "the Emergency Kit stays the same", true, func() error { return changeMasterPassword(ctx, ui, dest) }},
		{"Regenerate Secret Key", "the old Emergency Kit stops working", true, func() error { return regenerateSecretKey(ctx, ui, dest) }},
		{"Replace backend credentials", "new keys at the provider, then a new kit", true, func() error { return replaceBackendCredentials(ctx, ui, dest) }},
		{"Use a Recovery Code", "sets a new master password and Secret Key", false, func() error {
			return recoveryReset(ctx, ui, d, dest.Name, dest.Recover)
		}},
		{"Paste an updated Emergency Kit", "after a change on another computer", false, func() error {
			next, err := pasteUpdatedKit(ctx, ui, d)
			if err == nil && next.Name == dest.Name {
				dest = next
			}
			return err
		}},
	}
	cursor := 0
	for {
		i, err := ui.Pick(ctx, kitMenu(dest.Name, actions, cursor))
		if errors.Is(err, ErrBack) {
			return dest, nil
		}
		if err != nil {
			return dest, err
		}
		cursor = i
		if err := endAction(ui, actions[i].run()); err != nil {
			return dest, err
		}
	}
}

// kitMenu lists the actions, split by whether they ask for the master
// password.
func kitMenu(name string, actions []kitAction, cursor int) Picker {
	items := make([]Item, len(actions))
	for i, a := range actions {
		group := "WITHOUT IT"
		if a.unlocks {
			group = "NEEDS THE MASTER PASSWORD"
		}
		items[i] = Item{Group: group, Cells: []string{styles.Value.Render(a.label), styles.Dimmed.Render(a.detail)}}
	}
	return Picker{Page: kitPage(name, ""), Items: items, Cursor: cursor}
}

// kitPage is the page of Emergency Kit and password, or of one of its
// actions.
func kitPage(name, action string) Page {
	p := Page{Title: crumb(name, "Emergency Kit and password"),
		Subtitle: styles.Dimmed.Render("Emergency Kit and password for ") + styles.Value.Render(name)}
	if action != "" {
		p.Title = crumb(p.Title, action)
	}
	return p
}

func showKit(ctx context.Context, ui UI, dest *backup.Destination) error {
	page := kitPage(dest.Name, "Show Emergency Kit")
	a, err := unlockAccount(ctx, ui, dest, Page{Title: page.Title, Subtitle: subtitle("Show Emergency Kit", "Master password")}, "")
	if err != nil {
		return err
	}
	return ui.ShowKit(ctx, KitScreen{Page: page, Kit: a.Kit(),
		Lines: []Line{{Warning, "With the master password, this kit unlocks every backup: keep it private."}}})
}

func changeMasterPassword(ctx context.Context, ui UI, dest *backup.Destination) error {
	name := dest.Name
	if err := confirmAction(ctx, ui, Confirm{Question: "Change the master password for " + name + "?",
		Consequence: styles.Dimmed.Render("The Emergency Kit stays the same."), Yes: "Change it", Destructive: true}); err != nil {
		return err
	}
	title := kitPage(name, "Change master password").Title
	a, err := unlockAccount(ctx, ui, dest, Page{Title: title, Subtitle: subtitle("Change master password · step 1 of 2", "Master password")},
		"  "+styles.Dimmed.Render("First the current master password."))
	if err != nil {
		return err
	}
	master, err := askNewMasterPassword(ctx, ui, Page{Title: title, Subtitle: subtitle("Change master password · step 2 of 2", "New password")},
		"  "+styles.Dimmed.Render("Now the new one."), "Change it")
	if err != nil {
		return err
	}
	err = ui.Run(ctx, Running{Page: Page{Title: title, Subtitle: subtitle("Change master password", "Changing")},
		Label: "Changing it (this takes a few seconds)…", Detail: "re-keying the account key with the new password", ChangesKeysOn: name},
		func(ctx context.Context, _ func(Progress)) error { return a.ChangeMasterPassword(ctx, master) })
	if err != nil {
		return err
	}
	ui.Say(OK, "✓ The master password is changed. The Emergency Kit and every computer's key keep working.")
	return nil
}

// askNewMasterPassword asks for a new master password twice, until it's long
// enough and both match.
func askNewMasterPassword(ctx context.Context, ui UI, page Page, intro, submit string) (string, error) {
	f := Form{Page: page, Intro: intro, Submit: submit, Back: "Cancel", Fields: []Field{
		{Key: "master", Label: "NEW PASSWORD", Kind: Secret, Required: true, Validate: func(s string) string {
			if len(s) < 10 {
				return "Use at least 10 characters."
			}
			return ""
		}},
		{Key: "again", Label: "TYPE IT AGAIN", Kind: Secret, Required: true},
	}}
	for {
		v, err := ui.Form(ctx, f)
		if err != nil {
			return "", err
		}
		if v["master"] == v["again"] {
			return v["master"], nil
		}
		f.Fields[1].Error = "Those don't match; try again."
	}
}

func regenerateSecretKey(ctx context.Context, ui UI, dest *backup.Destination) error {
	name := dest.Name
	if err := confirmAction(ctx, ui, Confirm{Question: "Make a new Secret Key for " + name + "?",
		Consequence: styles.Dimmed.Render("The old Emergency Kit stops working, and you'll paste the new kit once on every other computer."),
		Yes:         "Make a new key", Destructive: true}); err != nil {
		return err
	}
	title := kitPage(name, "Regenerate Secret Key").Title
	a, err := unlockAccount(ctx, ui, dest, Page{Title: title, Subtitle: subtitle("Regenerate Secret Key", "Master password")}, "")
	if err != nil {
		return err
	}
	var kit string
	err = ui.Run(ctx, Running{Page: Page{Title: title, Subtitle: subtitle("Regenerate Secret Key", "Making it")},
		Label: "Making a new Secret Key (this takes a few seconds)…", Detail: "a new account key, then the old one removed", ChangesKeysOn: name},
		func(ctx context.Context, _ func(Progress)) error {
			var err error
			kit, err = a.RegenerateSecretKey(ctx)
			if kit != "" && err != nil {
				// The new key is in force; only the old one's removal failed.
				ui.Say(Warning, err.Error())
				return nil
			}
			return err
		})
	if err != nil {
		return err
	}
	return showNewKit(ctx, ui, Page{Title: title, Subtitle: subtitle("Regenerate Secret Key", "New Emergency Kit")}, kit)
}

// replaceBackendCredentials is Replace backend credentials: step 3 of I lost
// this computer on its own, for finishing that flow or rotating the
// credentials any time. It shows the new kit.
func replaceBackendCredentials(ctx context.Context, ui UI, dest *backup.Destination) error {
	name := dest.Name
	title := kitPage(name, "Replace backend credentials").Title
	page := func(step string) Page { return Page{Title: title, Subtitle: subtitle("Replace backend credentials", step)} }
	a, err := unlockAccount(ctx, ui, dest, page("Master password"), "")
	if err != nil {
		return err
	}
	if err := replaceCredentials(ctx, ui, a, dest, page, nil, "Cancel"); err != nil {
		return err
	}
	ui.Say(OK, "✓ This computer opens "+name+" with the new credentials.")
	return showNewKit(ctx, ui, page("New Emergency Kit"), a.Kit())
}

// showNewKit shows a kit that replaces the old one. Leaving it unsaved says
// where to see it again.
func showNewKit(ctx context.Context, ui UI, page Page, kit string) error {
	err := ui.ShowKit(ctx, KitScreen{Page: page, Kit: kit, Lines: newKitLines()})
	switch {
	case errors.Is(err, ErrBack):
		ui.Say(Warning, "The new kit wasn't shown to the end: choose Show Emergency Kit to save it.")
		return nil
	case err != nil:
		return err
	}
	ui.Say(Info, pasteOnOtherComputers)
	return nil
}

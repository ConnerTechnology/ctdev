// Package backupapp is `ctdev backup` in a terminal: one full-screen program
// for every interactive flow. Flows are sequential functions that run on
// their own goroutine and talk to the screen only through UI; each call shows
// a screen and blocks until it's answered.
package backupapp

import (
	"context"
	"errors"

	"github.com/ConnerTechnology/ctdev/ctdev/backup"
)

// UI is the screen as a flow sees it. Every call returns ErrCancelled once
// the program has quit or ctx is done.
type UI interface {
	// Dashboard shows a home screen with its action bar and returns the
	// action chosen. esc returns ErrBack.
	Dashboard(ctx context.Context, d Dashboard) (Action, error)
	// Pick shows a list and returns the index chosen. esc (or q, for a
	// top-level list) returns ErrBack.
	Pick(ctx context.Context, p Picker) (int, error)
	// Confirm asks a question in place of the footer. It returns false on
	// esc or n, or on any key but y when it's destructive. A Confirm with
	// Keep returns ErrBack on esc.
	Confirm(ctx context.Context, c Confirm) (bool, error)
	// Form shows fields and returns their values once every visible field
	// is valid. esc returns ErrBack.
	Form(ctx context.Context, f Form) (Values, error)
	// Notice shows steps to do elsewhere and returns on enter. esc returns
	// ErrBack.
	Notice(ctx context.Context, n Notice) error
	// ShowKit shows the Emergency Kit and Recovery Code once, and returns
	// once they're saved. Leaving without saving asks first; esc then
	// returns ErrBack.
	ShowKit(ctx context.Context, k KitScreen) error
	// Run calls fn with a spinner, or a progress bar once fn reports
	// progress. esc or ctrl+c cancels fn's context and Run returns ErrBack,
	// unless fn finished anyway; a Running with ChangesKeysOn can't be
	// cancelled.
	Run(ctx context.Context, r Running, fn func(ctx context.Context, progress func(Progress)) error) error
	// Say adds a result line to the next screen, shown until a key is
	// pressed, and printed again after the program exits. Never say a secret.
	Say(kind Kind, text string)
}

var (
	// ErrBack is esc: the step was left, and the flow goes back one step.
	ErrBack = errors.New("back")
	// ErrCancelled means the program has quit, so the flow should return.
	ErrCancelled = errors.New("cancelled")
)

// Progress is how far a backup has got.
type Progress = backup.BackupProgress

// Kind is how a line of text reads.
type Kind int

const (
	Info    Kind = iota // dim
	OK                  // green: a step that worked
	Warning             // yellow
	Error               // red
	Plain               // bright
	Heading             // orange and bold
)

// Line is one line of a message.
type Line struct {
	Kind Kind
	Text string
}

// Tone is a badge's color and glyph.
type Tone int

const (
	Healthy   Tone = iota // ✓ green
	Busy                  // ● blue
	Attention             // ▲ yellow
	Danger                // ✗ red
)

// Badge sums up a screen's state at the right end of its title line. The
// zero Badge shows none.
type Badge struct {
	Tone Tone
	Text string // upper case, without the glyph: "HEALTHY"
}

// Page is what a screen shows above its footer: the title line, the dim
// subtitle and the body. A request whose Page is zero keeps the page on
// screen, so a confirm or a running step appears in place under it.
type Page struct {
	Title    string // the folder, or a breadcrumb: "~/Projects/website › Backups"
	Context  string // dim beside the title: "you're in ~/Projects/website/src"
	Badge    Badge
	Subtitle string // styled by the caller
	Body     string // styled by the caller; see Row
}

// Dashboard is a home screen: the page, the state's message and the action
// bar. A Say line replaces Message until the next key.
type Dashboard struct {
	Page    Page
	Message []Line
	Actions []Action
	Focus   string // the key of the action focused first; "" is the first available
	Back    string // the esc item's label, last in the bar; "" draws none
	Help    string // "" is "←/→ choose · enter run · or press the letter"
}

// Action is one item of the action bar. Its Key is a single character,
// upper case for a destructive action. A Dimmed action is shown, but its key
// does nothing and the cursor skips it.
type Action struct {
	Key    string
	Label  string
	Dimmed bool
}

// Picker is a list to choose from. With no Page body it fills the body; under
// a page with a body it appears in place of the footer.
type Picker struct {
	Page    Page
	Heading string // orange line above the list
	Hint    string // dim beside the heading
	Intro   string // styled by the caller, above the list
	Columns []Column
	Items   []Item
	Cursor  int
	Radio   bool   // ◉ on the cursor row, ○ on the others
	Note    string // dim, under the list
	Enter   string // the enter button; "" is "Open"
	Back    string // the esc label; "" is "Back"
	Quit    bool   // a top-level list: q Quit instead of esc Back
	Help    string
}

// Column heads a column of a Picker; numbers are Right aligned.
type Column struct {
	Title string
	Right bool
}

// Item is a row of a Picker. Cells may be styled; the cursor row shows them
// plain, as one bar.
type Item struct {
	Cells  []string
	Group  string // a heading shown above the item when it differs from the one before
	Detail string // dim, under the item
	Dimmed bool   // shown, but the cursor skips it
}

// Confirm is a question in place of the footer. A safe one is answered with
// enter; a Destructive one only with y. TypeToConfirm adds a field the name
// must be typed into, exactly, before enter confirms.
type Confirm struct {
	Page          Page
	Warning       string // yellow, above the question
	Question      string
	Consequence   string // styled by the caller; plain text is bright
	Yes           string // the confirm button's label
	No            string // the esc label; "" is "Cancel"
	Destructive   bool
	TypeToConfirm string
	// Keep, when set, is an enter button that answers false; esc then
	// returns ErrBack, for a question with a third way out.
	Keep string
	Help string
}

// Values are a Form's answers by field key.
type Values map[string]string

// FieldKind is what a form field takes.
type FieldKind int

const (
	Text     FieldKind = iota // echoed
	Secret                    // masked as ••••••••••
	Choice                    // one of Options, chosen with ↑/↓
	Number                    // a whole number, 0 or more
	KitPaste                  // an Emergency Kit, pasted or typed, never echoed
)

// Field is one line of a Form.
type Field struct {
	Key      string
	Label    string // upper case
	Kind     FieldKind
	Value    string // the starting value; for a Choice, an option's Value
	Required bool
	Options  []Option // a Choice's
	// Validate returns what's wrong with a value, or "".
	Validate func(string) string
	// Show, when set, hides the field unless it returns true for the
	// form's current values, so a Choice can swap the fields below it.
	Show  func(Values) bool
	Error string // red under the field from the start: a later step's error
	Note  string // dim under the field
}

// Option is one choice of a Choice field.
type Option struct {
	Value string
	Label string
}

// Form is fields to fill in, one per line.
type Form struct {
	Page   Page
	Intro  string // styled by the caller, above the fields
	Fields []Field
	// Error is a red line above the footer, for an error a later step
	// returned about the form as a whole.
	Error  string
	Submit string // the enter button; "" is "Next"
	Back   string // the esc label; "" is "Back"
	Help   string
}

// Notice is steps the user does elsewhere, such as a provider's console.
type Notice struct {
	Page    Page
	Intro   string // styled by the caller, above the heading
	Heading string
	Steps   []string // numbered from 1
	Note    string   // dim, under the steps
	Ack     string   // the enter button
	Back    string   // the esc label; "" is "Back"
	Help    string   // "" is "press enter once that's done"
}

// KitScreen shows secrets once: the Emergency Kit and, when there is one,
// the Recovery Code.
type KitScreen struct {
	Page         Page
	Kit          string
	RecoveryCode string
	Lines        []Line // under the kit and code
	New          bool   // it replaces the old: headed "New Emergency Kit", "New Recovery Code"
	Ack          string // "" is "I've saved it", or "I've saved both" with a code
}

// Running is a slow call in progress.
type Running struct {
	Page   Page
	Label  string // "Checking the master password…"; the heading once progress arrives
	Detail string // dim: what it waits on
	// CancelLabel is the cancel key's label; "" is "Cancel".
	CancelLabel string
	// ChangesKeysOn names the destination whose keys the step changes: such
	// a step can't be cancelled, and says so.
	ChangesKeysOn string
}

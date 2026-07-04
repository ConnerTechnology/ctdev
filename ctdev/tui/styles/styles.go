package styles

import "charm.land/lipgloss/v2"

// Brand accents. These mid-tone hues read acceptably on both light and dark
// terminals, so they stay fixed regardless of the detected background.
var (
	Green  = lipgloss.Color("#3fb950")
	Red    = lipgloss.Color("#f85149")
	Yellow = lipgloss.Color("#d29922")
	Blue   = lipgloss.Color("#58a6ff")
	Orange = lipgloss.Color("#f0883e")
)

// Subtle (secondary text) and Bright (primary text) flip with the terminal
// background — near-white text is invisible on a light terminal and light-gray
// detail washes out. SetDarkBackground recomputes them; the defaults below are
// the dark-terminal values so styles are usable before detection runs and when
// output is not a TTY.
var (
	Subtle = lipgloss.Color("#8b949e")
	Bright = lipgloss.Color("#f0f6fc")
)

// fixedBright is an always-light foreground for self-contained dark chips (the
// cursor bar and the danger badge) so they stay legible on either theme — these
// must NOT follow the adaptive Bright, which goes near-black on light terminals.
var (
	fixedBright = lipgloss.Color("#f0f6fc")
	cursorBg    = lipgloss.Color("#161b22")
	borderGray  = lipgloss.Color("#30363d")
)

// Style variables are rebuilt whenever the palette changes (see SetDarkBackground).
var (
	Title    lipgloss.Style
	Subtitle lipgloss.Style
	Success  lipgloss.Style
	Error    lipgloss.Style
	Warning  lipgloss.Style
	Dimmed   lipgloss.Style

	Selected   lipgloss.Style
	Unselected lipgloss.Style
	Cursor     lipgloss.Style

	BadgeWarn   lipgloss.Style
	BadgeDanger lipgloss.Style

	// Header is the bold-orange accent shared by section and category headers.
	Header lipgloss.Style
	// CategoryHeader is an alias kept for the picker/progress call sites.
	CategoryHeader lipgloss.Style

	// Value styles the value half of a label/value pair. Pair it with Label,
	// which takes the column width since that varies per screen.
	Value lipgloss.Style

	StatusBar lipgloss.Style
	Help      lipgloss.Style
)

func init() { rebuild() }

// noColor drops every color from the palette (see Disable). Structure survives
// through bold/reverse, which NO_COLOR (no-color.org) explicitly allows.
var noColor bool

// Disable rebuilds every style without color — for NO_COLOR and for output
// that isn't a terminal, where raw ANSI codes would litter logs and pipes.
// Call it once at startup, before any rendering.
func Disable() {
	noColor = true
	rebuild()
}

// SetDarkBackground tunes the theme-dependent colors to the terminal background.
// Call it once at startup (see cmd.Execute) after detecting the background;
// isDark=true keeps the dark defaults.
func SetDarkBackground(isDark bool) {
	ld := lipgloss.LightDark(isDark)
	Subtle = ld(lipgloss.Color("#57606a"), lipgloss.Color("#8b949e"))
	Bright = ld(lipgloss.Color("#1f2328"), lipgloss.Color("#f0f6fc"))
	rebuild()
}

// rebuild reconstructs every style from the current palette. Call sites read
// these package vars live, so reassigning them before rendering takes effect.
func rebuild() {
	if noColor {
		rebuildPlain()
		return
	}
	Title = lipgloss.NewStyle().Bold(true).Foreground(Blue)
	Subtitle = lipgloss.NewStyle().Foreground(Subtle)
	Success = lipgloss.NewStyle().Foreground(Green)
	Error = lipgloss.NewStyle().Foreground(Red)
	Warning = lipgloss.NewStyle().Foreground(Yellow)
	Dimmed = lipgloss.NewStyle().Foreground(Subtle)

	Selected = lipgloss.NewStyle().Foreground(Green).SetString("◉")
	Unselected = lipgloss.NewStyle().Foreground(Subtle).SetString("○")
	// A self-contained dark chip with a fixed-light foreground, so the highlighted
	// row reads as a solid bar on either terminal theme.
	Cursor = lipgloss.NewStyle().Background(cursorBg).Foreground(fixedBright)

	// Severity badges. The uppercase label stays inside the colored block so the
	// meaning survives without color (accessibility); contrast is tuned per bg.
	BadgeWarn = lipgloss.NewStyle().Foreground(lipgloss.Color("#0d1117")).Background(Yellow).Bold(true).Padding(0, 1)
	BadgeDanger = lipgloss.NewStyle().Foreground(fixedBright).Background(Red).Bold(true).Padding(0, 1)

	Header = lipgloss.NewStyle().Bold(true).Foreground(Orange)
	CategoryHeader = Header

	Value = lipgloss.NewStyle().Foreground(Bright)

	StatusBar = lipgloss.NewStyle().
		Foreground(Subtle).
		BorderTop(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(borderGray).
		MarginTop(1).
		PaddingTop(1)

	Help = lipgloss.NewStyle().Foreground(Subtle)
}

// rebuildPlain is the colorless twin of rebuild. Selection and badges keep
// their meaning through glyphs, bold, and reverse video instead of hue.
func rebuildPlain() {
	plain := lipgloss.NewStyle()
	Title = plain.Bold(true)
	Subtitle = plain
	Success = plain
	Error = plain.Bold(true)
	Warning = plain
	Dimmed = plain

	Selected = plain.SetString("◉")
	Unselected = plain.SetString("○")
	Cursor = plain.Reverse(true)

	BadgeWarn = plain.Bold(true).Padding(0, 1).Reverse(true)
	BadgeDanger = BadgeWarn

	Header = plain.Bold(true)
	CategoryHeader = Header

	Value = plain

	StatusBar = plain.
		BorderTop(true).
		BorderStyle(lipgloss.NormalBorder()).
		MarginTop(1).
		PaddingTop(1)

	Help = plain
}

// Label styles the label half of a label/value pair at the given column width.
func Label(width int) lipgloss.Style {
	if noColor {
		return lipgloss.NewStyle().Width(width)
	}
	return lipgloss.NewStyle().Foreground(Subtle).Width(width)
}

package styles

import "charm.land/lipgloss/v2"

var (
	// Colors
	Green  = lipgloss.Color("#3fb950")
	Red    = lipgloss.Color("#f85149")
	Yellow = lipgloss.Color("#d29922")
	Blue   = lipgloss.Color("#58a6ff")
	Orange = lipgloss.Color("#f0883e")
	Subtle = lipgloss.Color("#8b949e")
	Bright = lipgloss.Color("#f0f6fc")

	// Text styles
	Title    = lipgloss.NewStyle().Bold(true).Foreground(Blue)
	Subtitle = lipgloss.NewStyle().Foreground(Subtle)
	Success  = lipgloss.NewStyle().Foreground(Green)
	Error    = lipgloss.NewStyle().Foreground(Red)
	Warning  = lipgloss.NewStyle().Foreground(Yellow)
	Dimmed   = lipgloss.NewStyle().Foreground(Subtle)

	// Component styles
	Selected   = lipgloss.NewStyle().Foreground(Green).SetString("◉")
	Unselected = lipgloss.NewStyle().Foreground(Subtle).SetString("○")
	Cursor     = lipgloss.NewStyle().Background(lipgloss.Color("#161b22"))

	// Severity badges. The uppercase label stays inside the colored block so the
	// meaning survives without color (accessibility); contrast is tuned per bg.
	BadgeWarn   = lipgloss.NewStyle().Foreground(lipgloss.Color("#0d1117")).Background(Yellow).Bold(true).Padding(0, 1)
	BadgeDanger = lipgloss.NewStyle().Foreground(Bright).Background(Red).Bold(true).Padding(0, 1)

	// Header is the bold-orange accent shared by section and category headers.
	Header = lipgloss.NewStyle().Bold(true).Foreground(Orange)
	// CategoryHeader is an alias kept for the picker/progress call sites.
	CategoryHeader = Header

	// Value styles the value half of a label/value pair. Pair it with Label,
	// which takes the column width since that varies per screen.
	Value = lipgloss.NewStyle().Foreground(Bright)

	// Status bar
	StatusBar = lipgloss.NewStyle().
			Foreground(Subtle).
			BorderTop(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("#30363d")).
			MarginTop(1).
			PaddingTop(1)

	// Help text
	Help = lipgloss.NewStyle().Foreground(Subtle)
)

// Label styles the label half of a label/value pair at the given column width.
func Label(width int) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(Subtle).Width(width)
}

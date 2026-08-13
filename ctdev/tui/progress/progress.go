package progress

import (
	"fmt"
	"strings"
	"time"

	bprogress "charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/styles"
)

type ComponentStatus int

const (
	StatusWaiting ComponentStatus = iota
	StatusRunning
	StatusDone
	StatusFailed
	StatusSkipped
)

type ComponentState struct {
	Name      string
	Status    ComponentStatus
	Output    []string
	Error     string
	Duration  time.Duration
	StartedAt time.Time
}

type tickMsg time.Time

func tickEvery() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// Messages
type InstallStartMsg struct{ Name string }
type InstallOutputMsg struct{ Name, Line string }
type InstallDoneMsg struct {
	Name     string
	Duration time.Duration
}
type InstallFailMsg struct {
	Name, Error string
	Duration    time.Duration
}
type InstallSkipMsg struct{ Name string }
type AllDoneMsg struct{}

type Mode int

const (
	ModeInstall Mode = iota
	ModeUninstall
	ModeUpdate
)

// outputTailMax is how many output lines are retained per component — enough
// that a failure's explanation (the apt/dpkg error, not just "exit status 1")
// survives into the summary. The live view shows only the last few.
const outputTailMax = 30

// liveTailLines is how many of the retained lines show under the running item.
const liveTailLines = 3

// failTailLines caps the output replayed under a failed item in the summary.
const failTailLines = 15

type Model struct {
	components  []ComponentState
	current     int
	spinner     spinner.Model
	progressBar bprogress.Model
	done        bool
	startTime   time.Time
	width       int
	height      int
	mode        Mode
	dryRun      bool
}

func New(names []string, mode Mode, dryRun bool) Model {
	s := spinner.New(
		spinner.WithSpinner(spinner.Dot),
		spinner.WithStyle(lipgloss.NewStyle().Foreground(styles.Orange)),
	)
	p := bprogress.New(
		bprogress.WithDefaultBlend(),
		bprogress.WithWidth(40),
	)

	states := make([]ComponentState, len(names))
	for i, name := range names {
		states[i] = ComponentState{Name: name, Status: StatusWaiting}
	}

	return Model{
		components:  states,
		spinner:     s,
		progressBar: p,
		startTime:   time.Now(),
		mode:        mode,
		dryRun:      dryRun,
	}
}

// Counts reports how many components ended in each terminal state and how many
// never ran (still waiting/running when the program exited — e.g. after a
// Ctrl-C). Callers use it to derive an honest exit code after Run returns.
func (inst *Model) Counts() (done, failed, skipped, notRun int) {
	for _, c := range inst.components {
		switch c.Status {
		case StatusDone:
			done++
		case StatusFailed:
			failed++
		case StatusSkipped:
			skipped++
		default:
			notRun++
		}
	}
	return done, failed, skipped, notRun
}

func (inst *Model) Init() tea.Cmd {
	// RequestBackgroundColor lets the palette adapt to a light terminal; the reply
	// arrives async as a tea.BackgroundColorMsg, so it never blocks startup.
	return tea.Batch(inst.spinner.Tick, tickEvery(), tea.RequestBackgroundColor)
}

func (inst *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		inst.width = msg.Width
		inst.height = msg.Height
		// Size the bar to the terminal, leaving room for the "N of M" suffix and
		// indent; clamp so it stays sane on very narrow or very wide terminals.
		inst.progressBar.SetWidth(clamp(msg.Width-16, 10, 80))
		return inst, nil
	case tea.BackgroundColorMsg:
		styles.SetDarkBackground(msg.IsDark())
		return inst, nil
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return inst, tea.Quit
		}
	case InstallStartMsg:
		for i := range inst.components {
			if inst.components[i].Name == msg.Name {
				inst.components[i].Status = StatusRunning
				inst.components[i].StartedAt = time.Now()
				inst.current = i
				break
			}
		}
	case InstallOutputMsg:
		for i := range inst.components {
			if inst.components[i].Name == msg.Name {
				inst.components[i].Output = appendTail(inst.components[i].Output, msg.Line, outputTailMax)
				break
			}
		}
	case InstallDoneMsg:
		for i := range inst.components {
			if inst.components[i].Name == msg.Name {
				inst.components[i].Status = StatusDone
				inst.components[i].Duration = msg.Duration
				break
			}
		}
		cmd := inst.progressBar.SetPercent(inst.donePercent())
		return inst, cmd
	case InstallFailMsg:
		for i := range inst.components {
			if inst.components[i].Name == msg.Name {
				inst.components[i].Status = StatusFailed
				inst.components[i].Error = msg.Error
				inst.components[i].Duration = msg.Duration
				break
			}
		}
		cmd := inst.progressBar.SetPercent(inst.donePercent())
		return inst, cmd
	case InstallSkipMsg:
		for i := range inst.components {
			if inst.components[i].Name == msg.Name {
				inst.components[i].Status = StatusSkipped
				break
			}
		}
		cmd := inst.progressBar.SetPercent(inst.donePercent())
		return inst, cmd
	case tickMsg:
		if !inst.done {
			return inst, tickEvery()
		}
	case AllDoneMsg:
		inst.done = true
		return inst, tea.Quit
	case spinner.TickMsg:
		var cmd tea.Cmd
		inst.spinner, cmd = inst.spinner.Update(msg)
		return inst, cmd
	case bprogress.FrameMsg:
		m, cmd := inst.progressBar.Update(msg)
		inst.progressBar = m
		return inst, cmd
	}
	return inst, nil
}

func (inst *Model) View() tea.View {
	var b strings.Builder

	if inst.done {
		b.WriteString(inst.viewSummary())
	} else {
		b.WriteString(inst.viewProgress())
	}

	return tea.NewView(b.String())
}

// action returns the in-progress verb for the mode.
func (inst *Model) action() string {
	switch inst.mode {
	case ModeUninstall:
		return "Uninstalling"
	case ModeUpdate:
		return "Updating"
	default:
		return "Installing"
	}
}

// dryRunSuffix labels the screen when nothing will actually change.
func (inst *Model) dryRunSuffix() string {
	if inst.dryRun {
		return " " + styles.Warning.Render("(dry run)")
	}
	return ""
}

func (inst *Model) viewProgress() string {
	var b strings.Builder

	doneCount := inst.countDone()
	total := len(inst.components)
	noun := "components"
	if inst.mode == ModeUpdate {
		noun = "updates"
	}
	b.WriteString(fmt.Sprintf("%s %d %s%s\n\n", inst.action(), total, noun, inst.dryRunSuffix()))
	b.WriteString(fmt.Sprintf("  %s  %d of %d\n\n", inst.progressBar.View(), doneCount, total))

	start, end, above, below := inst.componentWindow()
	if above > 0 {
		b.WriteString(fmt.Sprintf("  %s\n", styles.Dimmed.Render(fmt.Sprintf("↑ %d more", above))))
	}
	for _, c := range inst.components[start:end] {
		switch c.Status {
		case StatusDone:
			b.WriteString(fmt.Sprintf("  %s %s %s\n",
				styles.Success.Render("✓"),
				styles.Dimmed.Render(c.Name),
				styles.Dimmed.Render(fmt.Sprintf("%.1fs", c.Duration.Seconds())),
			))
		case StatusRunning:
			elapsed := time.Since(c.StartedAt).Truncate(time.Second)
			b.WriteString(fmt.Sprintf("  %s %s %s\n",
				inst.spinner.View(),
				lipgloss.NewStyle().Bold(true).Foreground(styles.Bright).Render(c.Name),
				styles.Dimmed.Render(fmt.Sprintf("(%s)", elapsed)),
			))
			for _, line := range tail(c.Output, liveTailLines) {
				b.WriteString(fmt.Sprintf("    %s\n", styles.Dimmed.Render(line)))
			}
		case StatusFailed:
			b.WriteString(fmt.Sprintf("  %s %s %s\n",
				styles.Error.Render("✗"),
				c.Name,
				styles.Error.Render(c.Error),
			))
		case StatusSkipped:
			b.WriteString(fmt.Sprintf("  %s %s %s\n",
				styles.Warning.Render("–"),
				styles.Dimmed.Render(c.Name),
				styles.Dimmed.Render("skipped"),
			))
		case StatusWaiting:
			b.WriteString(fmt.Sprintf("  %s %s\n",
				styles.Dimmed.Render("○"),
				styles.Dimmed.Render(c.Name),
			))
		}
	}
	if below > 0 {
		b.WriteString(fmt.Sprintf("  %s\n", styles.Dimmed.Render(fmt.Sprintf("↓ %d more", below))))
	}

	elapsed := time.Since(inst.startTime)
	b.WriteString(fmt.Sprintf("\n  Elapsed: %.1fs  ·  Ctrl+C to cancel\n", elapsed.Seconds()))
	return b.String()
}

// componentWindow slices the component list to what fits the terminal, keeping
// the running item in view — the view renders inline, so exceeding the screen
// height would garble the display on long lists.
func (inst *Model) componentWindow() (start, end, above, below int) {
	n := len(inst.components)
	// Chrome around the list: title+bar block (4 lines), footer (2), the running
	// item's output tail, and one line for each ↑/↓ indicator.
	capacity := inst.height - 6 - liveTailLines - 2
	if inst.height <= 0 || capacity >= n {
		return 0, n, 0, 0
	}
	if capacity < 3 {
		capacity = 3
	}
	start = inst.current - capacity/2
	if start > n-capacity {
		start = n - capacity
	}
	if start < 0 {
		start = 0
	}
	end = start + capacity
	if end > n {
		end = n
	}
	return start, end, start, n - end
}

const (
	// summaryFailedNamesMax caps how many failed names the final frame lists, so
	// a run where everything failed can't reintroduce an unbounded frame.
	summaryFailedNamesMax = 5
	// summaryReportMinItems is the run size below which the detailed report adds
	// nothing the final frame didn't already say.
	summaryReportMinItems = 2
)

// viewSummary is the last frame the TUI renders, and its height must stay
// bounded no matter how many steps ran. Bubble Tea's inline renderer drops lines
// from the *top* of a frame taller than the terminal, which would silently eat
// the headline — the one line people actually read. The per-item detail lives in
// SummaryReport instead, which the caller prints as ordinary stdout text after
// the program exits, where the terminal scrolls it normally.
//
// The trailing newline is load-bearing: Bubble Tea leaves the cursor at the
// start of the final line, so without it the printed report overwrites this.
func (inst *Model) viewSummary() string {
	var b strings.Builder
	succeeded, failed, skipped, _ := inst.Counts()

	// Don't claim success when something failed — the header is the one line
	// people read.
	noun := map[Mode]string{ModeInstall: "Installation", ModeUninstall: "Uninstall", ModeUpdate: "Update"}[inst.mode]
	if failed > 0 {
		b.WriteString(styles.Error.Render(fmt.Sprintf("✗ %s finished with %d failure(s)", noun, failed)) + inst.dryRunSuffix() + "\n")
	} else {
		b.WriteString(styles.Success.Render("✓ "+noun+" complete") + inst.dryRunSuffix() + "\n")
	}

	// Build the tally so a clean run reads "N succeeded" without an alarming
	// red "0 failed"; failed/skipped segments appear only when non-zero.
	segments := []string{styles.Success.Render(fmt.Sprintf("%d succeeded", succeeded))}
	if skipped > 0 {
		segments = append(segments, styles.Warning.Render(fmt.Sprintf("%d skipped", skipped)))
	}
	if failed > 0 {
		segments = append(segments, styles.Error.Render(fmt.Sprintf("%d failed", failed)))
	}
	b.WriteString("\n  " + strings.Join(segments, " · ") + "\n")

	if failedNames := inst.FailedNames(); len(failedNames) > 0 {
		shown := failedNames
		suffix := ""
		if len(shown) > summaryFailedNamesMax {
			shown = shown[:summaryFailedNamesMax]
			suffix = fmt.Sprintf(" (+%d more)", len(failedNames)-summaryFailedNamesMax)
		}
		b.WriteString(fmt.Sprintf("  %s %s%s\n",
			styles.Error.Render("Failed:"), strings.Join(shown, " "), styles.Dimmed.Render(suffix)))

		// The retry hint only makes sense where the names are CLI arguments.
		if inst.mode != ModeUpdate {
			retryCmd := "install"
			if inst.mode == ModeUninstall {
				retryCmd = "uninstall"
			}
			b.WriteString(fmt.Sprintf("\n  Retry: ctdev %s %s\n", retryCmd, strings.Join(failedNames, " ")))
		}
	}

	return b.String()
}

// FailedNames lists the components that failed, in run order.
func (inst *Model) FailedNames() []string {
	var names []string
	for _, c := range inst.components {
		if c.Status == StatusFailed {
			names = append(names, c.Name)
		}
	}
	return names
}

// SummaryReport is the full per-item result: every component with its status,
// and the replayed output tail for each failure. Callers print it to stdout
// after the Bubble Tea program has exited, so it lands in scrollback intact
// rather than being truncated by the inline renderer (see viewSummary).
//
// Returns "" when there is nothing worth printing — a short, entirely
// successful run is already fully described by the final frame.
func (inst *Model) SummaryReport() string {
	_, failed, skipped, _ := inst.Counts()
	if failed == 0 && skipped == 0 && len(inst.components) <= summaryReportMinItems {
		return ""
	}

	var b strings.Builder
	for _, c := range inst.components {
		switch c.Status {
		case StatusDone:
			b.WriteString(fmt.Sprintf("  %s %s %s\n",
				styles.Success.Render("✓"), c.Name,
				styles.Dimmed.Render(fmt.Sprintf("%.1fs", c.Duration.Seconds())),
			))
		case StatusFailed:
			b.WriteString(fmt.Sprintf("  %s %s %s\n",
				styles.Error.Render("✗"), c.Name,
				styles.Error.Render(c.Error),
			))
			// Replay the failure's output — "exit status 1" alone isn't actionable;
			// the apt/dpkg/compose lines that explain it are in the tail.
			for _, line := range tail(c.Output, failTailLines) {
				b.WriteString(fmt.Sprintf("      %s\n", styles.Dimmed.Render(line)))
			}
		case StatusSkipped:
			b.WriteString(fmt.Sprintf("  %s %s %s\n",
				styles.Warning.Render("–"), c.Name,
				styles.Dimmed.Render("skipped (unsupported OS)"),
			))
		}
	}
	if b.Len() == 0 {
		return ""
	}
	return b.String()
}

func (inst *Model) donePercent() float64 {
	done := inst.countDone()
	if len(inst.components) == 0 {
		return 1.0
	}
	return float64(done) / float64(len(inst.components))
}

func (inst *Model) countDone() int {
	count := 0
	for _, c := range inst.components {
		if c.Status == StatusDone || c.Status == StatusFailed || c.Status == StatusSkipped {
			count++
		}
	}
	return count
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func appendTail(lines []string, line string, max int) []string {
	lines = append(lines, line)
	if len(lines) > max {
		lines = lines[len(lines)-max:]
	}
	return lines
}

// tail returns the last max lines of lines.
func tail(lines []string, max int) []string {
	if len(lines) > max {
		return lines[len(lines)-max:]
	}
	return lines
}

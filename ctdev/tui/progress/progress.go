package progress

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	bprogress "charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
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
	ModeInstall   Mode = iota
	ModeUninstall
)

type Model struct {
	components  []ComponentState
	current     int
	spinner     spinner.Model
	progressBar bprogress.Model
	done        bool
	startTime   time.Time
	width       int
	mode        Mode
}

func New(names []string, mode Mode) Model {
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
	}
}

func (inst *Model) Init() tea.Cmd {
	return tea.Batch(inst.spinner.Tick, tickEvery())
}

func (inst *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		inst.width = msg.Width
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
				inst.components[i].Output = appendTail(inst.components[i].Output, msg.Line, 3)
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

func (inst *Model) viewProgress() string {
	var b strings.Builder

	doneCount := inst.countDone()
	total := len(inst.components)
	action := "Installing"
	if inst.mode == ModeUninstall {
		action = "Uninstalling"
	}
	b.WriteString(fmt.Sprintf("%s %d components\n\n", action, total))
	b.WriteString(fmt.Sprintf("  %s  %d of %d\n\n", inst.progressBar.View(), doneCount, total))

	for _, c := range inst.components {
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
			for _, line := range c.Output {
				b.WriteString(fmt.Sprintf("    %s\n", styles.Dimmed.Render(line)))
			}
		case StatusFailed:
			b.WriteString(fmt.Sprintf("  %s %s %s\n",
				styles.Error.Render("✗"),
				c.Name,
				styles.Error.Render(c.Error),
			))
		case StatusWaiting:
			b.WriteString(fmt.Sprintf("  %s %s\n",
				styles.Dimmed.Render("○"),
				styles.Dimmed.Render(c.Name),
			))
		}
	}

	elapsed := time.Since(inst.startTime)
	b.WriteString(fmt.Sprintf("\n  Elapsed: %.1fs  ·  Ctrl+C to cancel\n", elapsed.Seconds()))
	return b.String()
}

func (inst *Model) viewSummary() string {
	var b strings.Builder
	succeeded, failed := 0, 0
	var failedNames []string

	completeMsg := "✓ Installation complete"
	if inst.mode == ModeUninstall {
		completeMsg = "✓ Uninstall complete"
	}
	b.WriteString(styles.Success.Render(completeMsg) + "\n\n")

	for _, c := range inst.components {
		switch c.Status {
		case StatusDone:
			succeeded++
			b.WriteString(fmt.Sprintf("  %s %s %s\n",
				styles.Success.Render("✓"), c.Name,
				styles.Dimmed.Render(fmt.Sprintf("%.1fs", c.Duration.Seconds())),
			))
		case StatusFailed:
			failed++
			failedNames = append(failedNames, c.Name)
			b.WriteString(fmt.Sprintf("  %s %s %s\n",
				styles.Error.Render("✗"), c.Name,
				styles.Error.Render(c.Error),
			))
		case StatusSkipped:
			b.WriteString(fmt.Sprintf("  %s %s %s\n",
				styles.Warning.Render("–"), c.Name,
				styles.Dimmed.Render("skipped (unsupported OS)"),
			))
		}
	}

	b.WriteString(fmt.Sprintf("\n  %s · %s\n",
		styles.Success.Render(fmt.Sprintf("%d succeeded", succeeded)),
		styles.Error.Render(fmt.Sprintf("%d failed", failed)),
	))

	if len(failedNames) > 0 {
		retryCmd := "install"
		if inst.mode == ModeUninstall {
			retryCmd = "uninstall"
		}
		b.WriteString(fmt.Sprintf("\n  Retry: ctdev %s %s\n", retryCmd, strings.Join(failedNames, " ")))
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

func appendTail(lines []string, line string, max int) []string {
	lines = append(lines, line)
	if len(lines) > max {
		lines = lines[len(lines)-max:]
	}
	return lines
}

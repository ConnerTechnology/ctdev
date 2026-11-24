package backupapp

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/backupscreen"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/styles"
)

// running is a Running's step: a spinner, or a progress bar once progress
// arrives. It isn't answered; the flow moves on when its call returns.
type running struct {
	r          Running
	cancel     func()
	cancelling bool
	spin       spinner.Model
	bar        progress.Model
	prog       *Progress
}

func newRunning(r Running, cancel func()) *running {
	return &running{
		r:      r,
		cancel: cancel,
		spin:   spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(styles.Header)),
		bar:    progress.New(progress.WithDefaultBlend(), progress.WithoutPercentage(), progress.WithFillCharacters('█', '░')),
	}
}

func (rn *running) init() tea.Cmd { return rn.spin.Tick }

func (rn *running) setProgress(p Progress) { rn.prog = &p }

// ctrlC cancels like esc; a second one, once cancelling, quits.
func (rn *running) ctrlC() (*answer, tea.Cmd) {
	if rn.cancelling {
		return &answer{err: errQuit}, nil
	}
	return rn.stop(), nil
}

func (rn *running) stop() *answer {
	if rn.r.ChangesKeysOn == "" {
		rn.cancelling = true
		rn.cancel()
	}
	return nil
}

func (rn *running) update(msg tea.Msg) (*answer, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		rn.spin, cmd = rn.spin.Update(msg)
		return nil, cmd
	case tea.KeyPressMsg:
		if msg.String() == "esc" {
			return rn.stop(), nil
		}
	}
	return nil, nil
}

func (rn *running) view(width, _ int) stepView {
	r := rn.r
	var lines []string
	if p := rn.prog; p != nil {
		head := "  " + styles.Header.Render(r.Label)
		if r.Detail != "" {
			head += "  " + styles.Dimmed.Render(r.Detail)
		}
		rn.bar.SetWidth(max(width-12, 10))
		lines = append(lines, head, "",
			"  "+rn.bar.ViewAs(p.Percent)+"  "+styles.Value.Render(fmt.Sprintf("%d%%", int(p.Percent*100))),
			"  "+progressLine(*p))
		if p.CurrentFile != "" {
			lines = append(lines, "  "+styles.Dimmed.Render(strings.TrimPrefix(p.CurrentFile, "/")))
		}
	} else {
		lines = append(lines, r.Above...)
		lines = append(lines, "  "+rn.spin.View()+" "+styles.Value.Render(r.Label))
		lines = append(lines, r.Below...)
		indent := "    "
		if r.Above != nil || r.Below != nil {
			indent = "  " // under the list, not the spinner's line
		}
		if r.Detail != "" {
			lines = append(lines, indent+styles.Dimmed.Render(r.Detail))
		}
	}
	keys := "  " + keyHint("esc", orDefault(r.CancelLabel, "Cancel"))
	switch {
	case r.ChangesKeysOn != "":
		keys = "  " + styles.Dimmed.Render("Can't be cancelled: this step changes the keys on "+r.ChangesKeysOn+".")
	case rn.cancelling:
		keys = "  " + styles.Dimmed.Render("Cancelling…")
	}
	return stepView{content: lines, keys: keys}
}

// progressLine is how far a backup has got: files and bytes done of the
// totals, and the time left, until restic is still counting.
func progressLine(p Progress) string {
	dim, val := styles.Dimmed.Render, styles.Value.Render
	if p.SecondsLeft == 0 {
		return val(count(p.FilesDone)) + dim(" files  ·  ") + val(backupscreen.Bytes(p.BytesDone)) + dim("  ·  counting…")
	}
	return val(count(p.FilesDone)) + dim(" of "+count(p.FilesTotal)+" files  ·  ") +
		val(backupscreen.Bytes(p.BytesDone)) + dim(" of "+backupscreen.Bytes(p.BytesTotal)+"  ·  about "+left(p.SecondsLeft)+" left")
}

// count is a number with thousands separators.
func count(n int) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// left is a time remaining, roughly: "40s", "3m", "1h 5m".
func left(seconds int) string {
	d := time.Duration(seconds) * time.Second
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", seconds)
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Round(time.Minute).Minutes()))
	}
	return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
}

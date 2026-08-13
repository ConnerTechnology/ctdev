package diagnose

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/styles"
	"github.com/charmbracelet/x/ansi"
)

const (
	// nameWidth is the check-name column. Wide enough for "Internet reachable"
	// without pushing detail off an 80-column terminal.
	nameWidth = 22
	minWidth  = 60
	defWidth  = 80
)

// Render returns the whole report as a string. It's pure and width-parameterized
// so the command layer can print it, strip ANSI for a pipe, or hand it to a
// file — the same split package tui/info uses.
func Render(r Report, width int) string {
	if width < minWidth {
		width = defWidth
	}
	var b strings.Builder

	b.WriteString(styles.Title.Render("ctdev diagnose — " + r.Facts.Hostname))
	if r.Version != "" {
		b.WriteString(styles.Dimmed.Render("  (ctdev " + r.Version + ")"))
	}
	b.WriteString("\n")
	b.WriteString(styles.Dimmed.Render(subtitle(r)))
	b.WriteString("\n\n")

	b.WriteString(renderFindings(r, width))
	b.WriteString(renderGroups(r, width))
	b.WriteString(renderSummary(r))

	return b.String()
}

func subtitle(r Report) string {
	parts := []string{describePlatform(string(r.Facts.Platform.OS), r.Facts)}
	parts = append(parts, fmt.Sprintf("%d checks", len(r.Results)))
	if r.Elapsed > 0 {
		parts = append(parts, r.Elapsed.Round(10*time.Millisecond).String())
	}
	if r.Deep {
		parts = append(parts, "deep")
	}
	return strings.Join(parts, " · ")
}

func describePlatform(fallback string, f Facts) string {
	p := f.Platform
	if p.Distro != "" {
		if p.DistroVersion != "" {
			return p.Distro + " " + p.DistroVersion
		}
		return p.Distro
	}
	return fallback
}

// renderFindings prints the verdict block: the answer, before the evidence.
// Someone reading over your shoulder should get it from this section alone.
func renderFindings(r Report, width int) string {
	if len(r.Findings) == 0 {
		return ""
	}
	var b strings.Builder
	body := width - 5

	for _, fd := range r.Findings {
		style := severityStyle(fd.Severity)
		b.WriteString("  " + style.Render(fd.Severity.Glyph()+"  "+fd.Title) + "\n")
		if fd.Detail != "" {
			b.WriteString(indent(styles.Dimmed.Render(wrap(fd.Detail, body)), "     ") + "\n")
		}
		if fd.Action != "" {
			b.WriteString(hangingIndent(styles.Value.Render(wrap("→ "+fd.Action, body-2)), "     ", "  ") + "\n")
		}
		b.WriteString("\n")
	}
	return b.String()
}

// wrap breaks text to fit a column without padding it out to that width.
// lipgloss's Width() sets a minimum as well as a maximum, which would leave
// trailing spaces on every wrapped line — visible the moment anyone pastes the
// report somewhere.
func wrap(s string, width int) string {
	if width < 20 {
		width = 20
	}
	return ansi.Wordwrap(s, width, "")
}

func renderGroups(r Report, width int) string {
	var b strings.Builder
	byGroup := make(map[string][]Check)
	for _, c := range r.Checks {
		if _, ran := r.Results[c.ID]; !ran {
			continue
		}
		byGroup[c.Group] = append(byGroup[c.Group], c)
	}

	for _, group := range groupsInOrder(byGroup) {
		checks := byGroup[group]
		// Worst first inside a section — the eye should land on the problem.
		slices.SortStableFunc(checks, func(a, c Check) int {
			return r.Results[a.ID].Severity.Rank() - r.Results[c.ID].Severity.Rank()
		})

		b.WriteString(styles.Header.Render(group) + "\n")
		for _, c := range checks {
			b.WriteString(renderCheck(c, r.Results[c.ID], width))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func renderCheck(c Check, res Result, width int) string {
	var b strings.Builder
	style := severityStyle(res.Severity)

	b.WriteString("  " + style.Render(res.Severity.Glyph()) + " ")
	b.WriteString(styles.Label(nameWidth).Render(c.Name))
	b.WriteString(styles.Value.Render(res.Detail) + "\n")

	// Advice earns a line only when there's something to do about it.
	if res.Advice != "" && (res.Severity == Warn || res.Severity == Fail) {
		indentBy := strings.Repeat(" ", nameWidth+4)
		wrapped := wrap("→ "+res.Advice, width-nameWidth-8)
		b.WriteString(hangingIndent(styles.Dimmed.Render(wrapped), indentBy, "  ") + "\n")
	}
	return b.String()
}

func renderSummary(r Report) string {
	counts := map[Severity]int{}
	for _, res := range r.Results {
		counts[res.Severity]++
	}

	var parts []string
	for _, s := range []Severity{Fail, Warn, Info, OK, Skipped} {
		if n := counts[s]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, s))
		}
	}
	if len(parts) == 0 {
		return ""
	}

	line := strings.Join(parts, ", ")
	if counts[Fail] == 0 && counts[Warn] == 0 {
		return styles.Success.Render("Nothing needs attention") + styles.Dimmed.Render("  ("+line+")") + "\n"
	}
	return styles.Dimmed.Render(line) + "\n"
}

// groupsInOrder returns the groups present, canonical ones first in the order
// a technician works through them, then anything else alphabetically.
func groupsInOrder(byGroup map[string][]Check) []string {
	var out []string
	for _, g := range GroupOrder {
		if len(byGroup[g]) > 0 {
			out = append(out, g)
		}
	}
	var extra []string
	for g := range byGroup {
		if !slices.Contains(GroupOrder, g) {
			extra = append(extra, g)
		}
	}
	slices.Sort(extra)
	return append(out, extra...)
}

func severityStyle(s Severity) lipgloss.Style {
	switch s {
	case OK:
		return styles.Success
	case Warn:
		return styles.Warning
	case Fail:
		return styles.Error
	case Info:
		return styles.Value
	default:
		return styles.Dimmed
	}
}

// indent prefixes every line of s, including wrapped continuations.
func indent(s, prefix string) string {
	ls := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range ls {
		ls[i] = prefix + l
	}
	return strings.Join(ls, "\n")
}

// hangingIndent indents the first line by prefix and every continuation by
// prefix plus hang, so wrapped advice lines up under its own text rather than
// under the "→" that introduces it.
func hangingIndent(s, prefix, hang string) string {
	ls := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range ls {
		if i == 0 {
			ls[i] = prefix + l
			continue
		}
		ls[i] = prefix + hang + l
	}
	return strings.Join(ls, "\n")
}

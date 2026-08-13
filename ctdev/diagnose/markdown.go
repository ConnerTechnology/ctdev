package diagnose

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// Markdown renders the report as a document to hand to someone — pasted into a
// ticket, sent to a client, or kept as a record of what the machine looked like
// on the day you saw it.
//
// It carries the same findings as the terminal view but reads as prose rather
// than a dashboard, because the person opening it usually wasn't standing next
// to you when it ran.
func Markdown(r Report) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# Diagnostic report — %s\n\n", r.Facts.Hostname)
	writeMarkdownHeader(&b, r)

	if len(r.Findings) > 0 {
		b.WriteString("## What needs attention\n\n")
		for _, fd := range r.Findings {
			fmt.Fprintf(&b, "### %s %s\n\n", fd.Severity.Glyph(), fd.Title)
			if fd.Detail != "" {
				fmt.Fprintf(&b, "%s\n\n", fd.Detail)
			}
			if fd.Action != "" {
				fmt.Fprintf(&b, "**What to do:** %s\n\n", fd.Action)
			}
		}
	} else {
		b.WriteString("## What needs attention\n\nNothing. Every check either passed or was inconclusive; see below.\n\n")
	}

	writeMarkdownChecks(&b, r)
	writeMarkdownData(&b, r)

	b.WriteString("---\n\n")
	b.WriteString("*Produced by `ctdev doctor`. Every check is read-only — nothing on this machine was changed.*\n")
	return b.String()
}

func writeMarkdownHeader(b *strings.Builder, r Report) {
	row := func(label, value string) {
		if value != "" {
			fmt.Fprintf(b, "- **%s:** %s\n", label, value)
		}
	}

	when := r.Started
	if when.IsZero() {
		when = time.Now()
	}
	row("Generated", when.Format("2006-01-02 15:04 MST"))
	row("ctdev", r.Version)
	row("System", describePlatform(string(r.Facts.Platform.OS), r.Facts)+" ("+string(r.Facts.Platform.OS)+"/"+r.Facts.Platform.Arch+")")

	if r.Facts.Iface != "" {
		row("Network interface", describeLink(r.Facts))
	}
	if r.Facts.LocalIP.IsValid() {
		row("Address", r.Facts.LocalIP.String())
	}
	if r.Facts.Gateway.IsValid() {
		row("Gateway", r.Facts.Gateway.String())
	}
	if len(r.Facts.DNS) > 0 {
		names := make([]string, 0, len(r.Facts.DNS))
		for _, d := range r.Facts.DNS {
			names = append(names, d.String())
		}
		row("DNS servers", strings.Join(names, ", "))
	}
	if r.Deep {
		row("Mode", "deep")
	}
	b.WriteString("\n")
}

func writeMarkdownChecks(b *strings.Builder, r Report) {
	byGroup := make(map[string][]Check)
	for _, c := range r.Checks {
		if _, ran := r.Results[c.ID]; ran {
			byGroup[c.Group] = append(byGroup[c.Group], c)
		}
	}

	for _, group := range groupsInOrder(byGroup) {
		checks := byGroup[group]
		slices.SortStableFunc(checks, func(a, c Check) int {
			return r.Results[a.ID].Severity.Rank() - r.Results[c.ID].Severity.Rank()
		})

		fmt.Fprintf(b, "## %s\n\n", group)
		b.WriteString("| | Check | Finding |\n|---|---|---|\n")
		for _, c := range checks {
			res := r.Results[c.ID]
			fmt.Fprintf(b, "| %s | %s | %s |\n",
				res.Severity.Glyph(), escapePipes(c.Name), escapePipes(res.Detail))
		}
		b.WriteString("\n")

		// Advice goes below the table rather than in a fourth column, which
		// would make every row unreadably wide.
		for _, c := range checks {
			res := r.Results[c.ID]
			if res.Advice != "" && (res.Severity == Warn || res.Severity == Fail) {
				fmt.Fprintf(b, "- **%s:** %s\n", escapePipes(c.Name), res.Advice)
			}
		}
		b.WriteString("\n")
	}
}

// writeMarkdownData appends the structured extras some checks collect — the
// per-resolver timings, the Wi-Fi particulars — for whoever reads this after
// the fact and wants the numbers rather than the summary.
func writeMarkdownData(b *strings.Builder, r Report) {
	var ids []string
	for id, res := range r.Results {
		if len(visibleData(res.Data)) > 0 {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return
	}
	slices.Sort(ids)

	b.WriteString("## Measurements\n\n")
	for _, id := range ids {
		fmt.Fprintf(b, "**%s**\n\n", id)
		data := visibleData(r.Results[id].Data)
		keys := make([]string, 0, len(data))
		for k := range data {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, k := range keys {
			fmt.Fprintf(b, "- %s: %s\n", k, data[k])
		}
		b.WriteString("\n")
	}
}

// secretishKeys are dropped from any rendered output. Nothing in this package
// puts a credential in Data today, and the vendor integrations must not either
// — but a report is a file people forward, so this is enforced at the render
// boundary rather than left to everyone who ever adds a check.
var secretishKeys = []string{"key", "token", "secret", "password", "passwd", "credential", "auth"}

func visibleData(data map[string]string) map[string]string {
	if len(data) == 0 {
		return nil
	}
	out := make(map[string]string, len(data))
	for k, v := range data {
		if v == "" || isSecretish(k) {
			continue
		}
		out[k] = v
	}
	return out
}

func isSecretish(key string) bool {
	lower := strings.ToLower(key)
	for _, s := range secretishKeys {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}

// escapePipes keeps a stray "|" in a detail string from breaking the table.
func escapePipes(s string) string {
	return strings.ReplaceAll(s, "|", `\|`)
}

// ReportFilename is the default name for a saved report: identifiable by
// machine and moment, and safe on every filesystem.
func ReportFilename(r Report) string {
	when := r.Started
	if when.IsZero() {
		when = time.Now()
	}
	host := r.Facts.Hostname
	if host == "" {
		host = "unknown"
	}
	return fmt.Sprintf("ctdev-doctor-%s-%s.md", safeFilename(host), when.Format("20060102-150405"))
}

func safeFilename(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return b.String()
}

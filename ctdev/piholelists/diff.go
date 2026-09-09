package piholelists

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

// Op is what a Change does to the Pi-hole.
type Op int

const (
	Add Op = iota
	Update
	Remove
)

func (inst Op) String() string {
	switch inst {
	case Add:
		return "ADD"
	case Update:
		return "UPDATE"
	default:
		return "REMOVE"
	}
}

// Change is one difference between the file and the Pi-hole. Entry is the state
// to end up in — for a Remove that is the live entry, so Kind and Key always
// identify the row regardless of Op. Live is what is on the Pi-hole now, nil
// for an Add.
type Change struct {
	Op    Op
	Entry Entry
	Live  *Entry
}

type entryID struct {
	kind Kind
	key  string
}

// Diff reports what applying the file to the Pi-hole would change, ordered by
// kind then key so the picker and a --dry-run print read the same way twice.
func Diff(file, live Lists) []Change {
	liveByID := map[entryID]Entry{}
	for _, e := range live {
		liveByID[entryID{e.Kind, e.Key}] = e
	}

	var changes []Change
	inFile := map[entryID]bool{}
	for _, want := range file {
		id := entryID{want.Kind, want.Key}
		inFile[id] = true
		have, ok := liveByID[id]
		if !ok {
			changes = append(changes, Change{Op: Add, Entry: want})
			continue
		}
		if !sameEntry(want, have) {
			changes = append(changes, Change{Op: Update, Entry: want, Live: &have})
		}
	}
	for _, have := range live {
		if !inFile[entryID{have.Kind, have.Key}] {
			changes = append(changes, Change{Op: Remove, Entry: have, Live: &have})
		}
	}

	slices.SortFunc(changes, func(a, b Change) int {
		if a.Entry.Kind != b.Entry.Kind {
			return int(a.Entry.Kind) - int(b.Entry.Kind)
		}
		return strings.Compare(a.Entry.Key, b.Entry.Key)
	})
	return changes
}

func sameEntry(a, b Entry) bool {
	return a.Comment == b.Comment && a.Enabled == b.Enabled && sameGroups(a.Groups, b.Groups)
}

func sameGroups(a, b []string) bool {
	return slices.Equal(normalizeGroups(a), normalizeGroups(b))
}

// NeedsGravity reports whether the changes require `pihole -g`. Only adlists do;
// domainlist edits are picked up by the much cheaper `pihole reloadlists`.
func NeedsGravity(changes []Change) bool {
	for _, c := range changes {
		if c.Entry.Kind == Adlist {
			return true
		}
	}
	return false
}

// Apply writes the changes to gravity.db in one transaction. The caller then
// reloads Pi-hole (see NeedsGravity) — nothing here restarts anything.
func Apply(ctx context.Context, ex Executor, changes []Change, liveGroups []string) error {
	if len(changes) == 0 {
		return nil
	}
	return ex.Exec(ctx, ApplySQL(changes, liveGroups))
}

// ApplySQL renders the changes as a single BEGIN/COMMIT script, creating any
// group the file references that the Pi-hole doesn't have yet. It is a plain
// string so it can be printed, diffed and tested without a Pi-hole.
func ApplySQL(changes []Change, liveGroups []string) string {
	var b strings.Builder
	b.WriteString("BEGIN TRANSACTION;\n")

	for _, name := range missingGroups(changes, liveGroups) {
		fmt.Fprintf(&b, "INSERT INTO \"group\" (name,enabled) VALUES (%s,1);\n", quote(name))
	}
	for _, c := range changes {
		writeChange(&b, c)
	}

	b.WriteString("COMMIT;\n")
	return b.String()
}

func missingGroups(changes []Change, liveGroups []string) []string {
	var missing []string
	for _, c := range changes {
		if c.Op == Remove {
			continue
		}
		for _, g := range c.Entry.Groups {
			if !slices.Contains(liveGroups, g) && !slices.Contains(missing, g) {
				missing = append(missing, g)
			}
		}
	}
	slices.Sort(missing)
	return missing
}

// target names the table a Kind lives in and the row's WHERE clause, so the
// domainlist and adlist halves of every statement share one code path.
type target struct {
	table   string
	link    string
	linkCol string
	where   string
}

func targetFor(e Entry) target {
	if e.Kind == Adlist {
		return target{
			table:   "adlist",
			link:    "adlist_by_group",
			linkCol: "adlist_id",
			where:   "address=" + quote(e.Key),
		}
	}
	return target{
		table:   "domainlist",
		link:    "domainlist_by_group",
		linkCol: "domainlist_id",
		where:   fmt.Sprintf("type=%d AND domain=%s", e.Kind.DomainlistType(), quote(e.Key)),
	}
}

func writeChange(b *strings.Builder, c Change) {
	t := targetFor(c.Entry)
	switch c.Op {
	case Add:
		if c.Entry.Kind == Adlist {
			fmt.Fprintf(b, "INSERT INTO adlist (address,enabled,comment) VALUES (%s,%s,%s);\n",
				quote(c.Entry.Key), boolValue(c.Entry.Enabled), nullableQuote(c.Entry.Comment))
		} else {
			fmt.Fprintf(b, "INSERT INTO domainlist (type,domain,enabled,comment) VALUES (%d,%s,%s,%s);\n",
				c.Entry.Kind.DomainlistType(), quote(c.Entry.Key), boolValue(c.Entry.Enabled), nullableQuote(c.Entry.Comment))
		}
		// The insert trigger links the new row to group 0 (Default), so only a
		// different group set needs the links rewritten.
		if !isDefaultGroups(c.Entry.Groups) {
			writeGroupLinks(b, t, c.Entry.Groups)
		}
	case Update:
		fmt.Fprintf(b, "UPDATE %s SET enabled=%s, comment=%s WHERE %s;\n",
			t.table, boolValue(c.Entry.Enabled), nullableQuote(c.Entry.Comment), t.where)
		if c.Live == nil || !sameGroups(c.Entry.Groups, c.Live.Groups) {
			writeGroupLinks(b, t, c.Entry.Groups)
		}
	case Remove:
		fmt.Fprintf(b, "DELETE FROM %s WHERE %s;\n", t.table, t.where)
	}
}

// writeGroupLinks replaces a row's group membership wholesale: the delete
// clears whatever is there (including the trigger's automatic Default link) and
// one insert per wanted group puts back exactly the file's set.
func writeGroupLinks(b *strings.Builder, t target, groups []string) {
	rowID := fmt.Sprintf("(SELECT id FROM %s WHERE %s)", t.table, t.where)
	fmt.Fprintf(b, "DELETE FROM %s WHERE %s IN (SELECT id FROM %s WHERE %s);\n", t.link, t.linkCol, t.table, t.where)
	for _, g := range groups {
		fmt.Fprintf(b, "INSERT INTO %s (%s,group_id) SELECT %s, id FROM \"group\" WHERE name=%s;\n",
			t.link, t.linkCol, rowID, quote(g))
	}
}

func boolValue(enabled bool) string {
	if enabled {
		return "1"
	}
	return "0"
}

func nullableQuote(s string) string {
	if s == "" {
		return "NULL"
	}
	return quote(s)
}

func quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

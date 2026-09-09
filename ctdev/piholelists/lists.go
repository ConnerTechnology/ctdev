// Package piholelists version-controls a Pi-hole's allow/deny/regex lists and
// adlists. Those live only in gravity.db, so this package models them as a
// single TOML file, reads the live state out of gravity.db through an Executor,
// diffs the two, and turns the differences into one SQL transaction.
//
// Everything here is pure logic over an injected Executor: nothing shells out,
// so the whole package is unit-testable without a Pi-hole.
package piholelists

import (
	"cmp"
	"slices"
	"strings"
)

// DefaultGroup is the group Pi-hole links every new entry to (id 0). Entries
// that omit "groups" belong to it.
const DefaultGroup = "Default"

// Kind is one of the five lists, which map onto two gravity.db tables: adlist,
// and domainlist keyed by its type column.
type Kind int

const (
	Adlist Kind = iota
	Allow
	Deny
	AllowRegex
	DenyRegex
)

// Kinds is every Kind in the order they appear in the TOML file and the picker.
var Kinds = []Kind{Adlist, Allow, Deny, AllowRegex, DenyRegex}

// Key is the TOML table name for this list.
func (inst Kind) Key() string {
	switch inst {
	case Adlist:
		return "adlists"
	case Allow:
		return "allow"
	case Deny:
		return "deny"
	case AllowRegex:
		return "allow_regex"
	default:
		return "deny_regex"
	}
}

// Field is the TOML key holding an entry's identity.
func (inst Kind) Field() string {
	switch inst {
	case Adlist:
		return "url"
	case Allow, Deny:
		return "domain"
	default:
		return "pattern"
	}
}

// Label is the human name used in the picker and in printed output.
func (inst Kind) Label() string {
	switch inst {
	case Adlist:
		return "Adlists"
	case Allow:
		return "Allow (exact)"
	case Deny:
		return "Deny (exact)"
	case AllowRegex:
		return "Allow regex"
	default:
		return "Deny regex"
	}
}

// DomainlistType is the domainlist.type value for this Kind, or -1 for adlists,
// which live in their own table.
func (inst Kind) DomainlistType() int {
	switch inst {
	case Adlist:
		return -1
	case Allow:
		return 0
	case Deny:
		return 1
	case AllowRegex:
		return 2
	default:
		return 3
	}
}

// Entry is one list member. Groups is normalized (sorted, deduped); nil means
// the entry belongs to no group at all, which Pi-hole treats as inactive.
type Entry struct {
	Kind    Kind
	Key     string
	Comment string
	Groups  []string
	Enabled bool
}

// Lists is a set of entries, unique by (Kind, Key) — gravity.db enforces the
// same uniqueness.
type Lists []Entry

// OfKind returns the entries of one Kind, in the order they appear.
func (inst Lists) OfKind(k Kind) Lists {
	var out Lists
	for _, e := range inst {
		if e.Kind == k {
			out = append(out, e)
		}
	}
	return out
}

// Sort orders entries by Kind then Key, so exports and diffs are stable.
func (inst Lists) Sort() {
	slices.SortFunc(inst, func(a, b Entry) int {
		if c := cmp.Compare(a.Kind, b.Kind); c != 0 {
			return c
		}
		return cmp.Compare(a.Key, b.Key)
	})
}

// normalizeGroups sorts and dedupes a group list and collapses "no groups" to
// nil, so a set comparison is a plain slice comparison.
func normalizeGroups(groups []string) []string {
	var out []string
	for _, g := range groups {
		g = strings.TrimSpace(g)
		if g != "" && !slices.Contains(out, g) {
			out = append(out, g)
		}
	}
	slices.Sort(out)
	return out
}

func isDefaultGroups(groups []string) bool {
	return len(groups) == 1 && groups[0] == DefaultGroup
}

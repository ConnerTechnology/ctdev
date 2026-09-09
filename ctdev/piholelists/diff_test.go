package piholelists

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestDiffClassifiesEveryChange(t *testing.T) {
	file := Lists{
		{Kind: Adlist, Key: "https://example.com/new.txt", Groups: []string{"Default"}, Enabled: true},
		{Kind: Allow, Key: "a.example", Comment: "new comment", Groups: []string{"Default"}, Enabled: true},
		{Kind: Deny, Key: "b.example", Groups: []string{"Default"}, Enabled: true},
		{Kind: DenyRegex, Key: `^kids\.`, Groups: []string{"Kids", "Default"}, Enabled: true},
	}
	live := Lists{
		{Kind: Allow, Key: "a.example", Comment: "old comment", Groups: []string{"Default"}, Enabled: true},
		{Kind: Deny, Key: "b.example", Groups: []string{"Default"}, Enabled: true},
		{Kind: Deny, Key: "gone.example", Groups: []string{"Default"}, Enabled: true},
		{Kind: DenyRegex, Key: `^kids\.`, Groups: []string{"Default", "Kids"}, Enabled: true},
	}

	changes := Diff(file, live)
	if len(changes) != 3 {
		t.Fatalf("expected 3 changes, got %d: %+v", len(changes), changes)
	}
	// Ordered by kind then key: the adlist add, the allow update, the deny remove.
	if changes[0].Op != Add || changes[0].Entry.Kind != Adlist {
		t.Errorf("change 0 = %+v, want an adlist Add", changes[0])
	}
	if changes[1].Op != Update || changes[1].Entry.Key != "a.example" {
		t.Errorf("change 1 = %+v, want an update to a.example", changes[1])
	}
	if changes[1].Live == nil || changes[1].Live.Comment != "old comment" {
		t.Errorf("an update must carry the live entry it replaces, got %+v", changes[1].Live)
	}
	if changes[2].Op != Remove || changes[2].Entry.Key != "gone.example" {
		t.Errorf("change 2 = %+v, want gone.example removed", changes[2])
	}
	if changes[2].Entry.Kind != Deny {
		t.Errorf("a remove must describe the live entry, got kind %v", changes[2].Entry.Kind)
	}
}

func TestDiffSpotsGroupAndEnabledDrift(t *testing.T) {
	live := Lists{{Kind: Allow, Key: "a.example", Groups: []string{"Default"}, Enabled: true}}
	for name, entry := range map[string]Entry{
		"groups":  {Kind: Allow, Key: "a.example", Groups: []string{"Kids"}, Enabled: true},
		"enabled": {Kind: Allow, Key: "a.example", Groups: []string{"Default"}},
	} {
		changes := Diff(Lists{entry}, live)
		if len(changes) != 1 || changes[0].Op != Update {
			t.Errorf("%s drift: expected one update, got %+v", name, changes)
		}
	}
}

func TestApplySQLAddsWithDefaultGroupWithoutTouchingLinks(t *testing.T) {
	sql := ApplySQL([]Change{
		{Op: Add, Entry: Entry{Kind: Allow, Key: "a.example", Groups: []string{"Default"}, Enabled: true}},
	}, []string{"Default"})

	want := "INSERT INTO domainlist (type,domain,enabled,comment) VALUES (0,'a.example',1,NULL);"
	if !strings.Contains(sql, want) {
		t.Errorf("missing %q in:\n%s", want, sql)
	}
	if strings.Contains(sql, "domainlist_by_group") {
		t.Errorf("the insert trigger already links group 0; no link SQL expected:\n%s", sql)
	}
	if !strings.HasPrefix(sql, "BEGIN TRANSACTION;\n") || !strings.HasSuffix(sql, "COMMIT;\n") {
		t.Errorf("the script must be one transaction:\n%s", sql)
	}
}

func TestApplySQLCreatesMissingGroupsAndRelinks(t *testing.T) {
	sql := ApplySQL([]Change{
		{Op: Add, Entry: Entry{Kind: DenyRegex, Key: `^kids\.`, Groups: []string{"Kids"}, Enabled: true}},
	}, []string{"Default"})

	for _, want := range []string{
		`INSERT INTO "group" (name,enabled) VALUES ('Kids',1);`,
		`INSERT INTO domainlist (type,domain,enabled,comment) VALUES (3,'^kids\.',1,NULL);`,
		`DELETE FROM domainlist_by_group WHERE domainlist_id IN (SELECT id FROM domainlist WHERE type=3 AND domain='^kids\.');`,
		`INSERT INTO domainlist_by_group (domainlist_id,group_id) SELECT (SELECT id FROM domainlist WHERE type=3 AND domain='^kids\.'), id FROM "group" WHERE name='Kids';`,
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("missing %q in:\n%s", want, sql)
		}
	}
}

func TestApplySQLUpdateAndRemove(t *testing.T) {
	live := Entry{Kind: Adlist, Key: "https://example.com/a.txt", Comment: "old", Groups: []string{"Default"}, Enabled: true}
	sql := ApplySQL([]Change{
		{Op: Update, Entry: Entry{Kind: Adlist, Key: live.Key, Comment: "new", Groups: []string{"Default"}, Enabled: true}, Live: &live},
		{Op: Remove, Entry: Entry{Kind: Deny, Key: "gone.example"}},
	}, []string{"Default"})

	for _, want := range []string{
		"UPDATE adlist SET enabled=1, comment='new' WHERE address='https://example.com/a.txt';",
		"DELETE FROM domainlist WHERE type=1 AND domain='gone.example';",
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("missing %q in:\n%s", want, sql)
		}
	}
	if strings.Contains(sql, "adlist_by_group") {
		t.Errorf("the groups did not change, so the links must be left alone:\n%s", sql)
	}
}

func TestApplySQLEscapesQuotes(t *testing.T) {
	sql := ApplySQL([]Change{
		{Op: Add, Entry: Entry{Kind: Allow, Key: "o'hara.example", Comment: "it's fine", Enabled: true, Groups: []string{"Default"}}},
	}, []string{"Default"})
	if !strings.Contains(sql, "VALUES (0,'o''hara.example',1,'it''s fine');") {
		t.Errorf("single quotes must be doubled:\n%s", sql)
	}
}

func TestNeedsGravityOnlyForAdlists(t *testing.T) {
	domainOnly := []Change{{Op: Add, Entry: Entry{Kind: Deny, Key: "a.example"}}}
	if NeedsGravity(domainOnly) {
		t.Error("domainlist changes reload without a gravity rebuild")
	}
	if !NeedsGravity(append(domainOnly, Change{Op: Remove, Entry: Entry{Kind: Adlist, Key: "https://x"}})) {
		t.Error("an adlist change needs gravity rebuilt")
	}
}

func TestChangeDetailNamesWhatDiffers(t *testing.T) {
	live := Entry{Kind: Allow, Key: "a.example", Comment: "old", Groups: []string{"Default"}, Enabled: true}
	cases := []struct {
		name   string
		change Change
		want   string
	}{
		{"add with comment", Change{Op: Add, Entry: Entry{Kind: Allow, Key: "a.example", Comment: "why", Groups: []string{"Default"}, Enabled: true}}, "why"},
		{"add in a group", Change{Op: Add, Entry: Entry{Kind: Deny, Key: "a.example", Groups: []string{"Kids"}, Enabled: true}}, "groups Kids"},
		{"comment change", Change{Op: Update, Entry: Entry{Kind: Allow, Key: "a.example", Comment: "new", Groups: []string{"Default"}, Enabled: true}, Live: &live}, `comment "old" → "new"`},
		{"group change", Change{Op: Update, Entry: Entry{Kind: Allow, Key: "a.example", Comment: "old", Groups: []string{"Kids"}, Enabled: true}, Live: &live}, "groups Default → Kids"},
		{"disabled", Change{Op: Update, Entry: Entry{Kind: Allow, Key: "a.example", Comment: "old", Groups: []string{"Default"}}, Live: &live}, "enabled → off"},
		{"remove", Change{Op: Remove, Entry: live, Live: &live}, "old"},
	}
	for _, c := range cases {
		if got := c.change.Detail(); got != c.want {
			t.Errorf("%s: Detail() = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestApplyRunsOneScriptAndSkipsEmptyChanges(t *testing.T) {
	ex := &fakeExec{}
	if err := Apply(context.Background(), ex, nil, nil); err != nil {
		t.Fatalf("Apply with no changes: %v", err)
	}
	if len(ex.execs) != 0 {
		t.Fatalf("nothing to apply should not touch the database, got %v", ex.execs)
	}

	changes := []Change{{Op: Add, Entry: Entry{Kind: Allow, Key: "a.example", Groups: []string{"Default"}, Enabled: true}}}
	if err := Apply(context.Background(), ex, changes, []string{"Default"}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(ex.execs) != 1 || !strings.Contains(ex.execs[0], "a.example") {
		t.Errorf("expected one script mentioning a.example, got %v", ex.execs)
	}

	failing := &fakeExec{err: errors.New("locked")}
	if err := Apply(context.Background(), failing, changes, nil); err == nil {
		t.Error("expected the executor error to propagate")
	}
}

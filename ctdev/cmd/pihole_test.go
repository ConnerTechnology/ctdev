package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ConnerTechnology/dotfiles/ctdev/piholelists"
)

func TestLoadListsReadsTheEmbeddedFileByDefault(t *testing.T) {
	lists, err := loadLists("")
	if err != nil {
		t.Fatalf("loadLists(embedded): %v", err)
	}
	if len(lists) == 0 {
		t.Fatal("the embedded lists.toml should not be empty")
	}

	path := filepath.Join(t.TempDir(), "lists.toml")
	if err := os.WriteFile(path, []byte(`allow = [ { domain = "a.example" } ]`), 0o644); err != nil {
		t.Fatal(err)
	}
	fromFile, err := loadLists(path)
	if err != nil {
		t.Fatalf("loadLists(%s): %v", path, err)
	}
	if len(fromFile) != 1 || fromFile[0].Key != "a.example" {
		t.Errorf("--from should win over the embedded file, got %+v", fromFile)
	}
	if _, err := loadLists(filepath.Join(t.TempDir(), "missing.toml")); err == nil {
		t.Error("a missing --from file should be an error")
	}
}

func TestPrintDiffLabelsEveryChange(t *testing.T) {
	live := piholelists.Entry{Kind: piholelists.Allow, Key: "a.example", Comment: "old", Groups: []string{"Default"}, Enabled: true}
	var out strings.Builder
	printDiff(&out, []piholelists.Change{
		{Op: piholelists.Add, Entry: piholelists.Entry{Kind: piholelists.Adlist, Key: "https://example.com/ads.txt", Comment: "new list"}},
		{Op: piholelists.Update, Entry: piholelists.Entry{Kind: piholelists.Allow, Key: "a.example", Comment: "new", Groups: []string{"Default"}, Enabled: true}, Live: &live},
		{Op: piholelists.Remove, Entry: piholelists.Entry{Kind: piholelists.Deny, Key: "gone.example"}},
	})

	got := out.String()
	for _, want := range []string{"Adlists", "Allow (exact)", "Deny (exact)", "ADD", "UPDATE", "REMOVE", "https://example.com/ads.txt", `comment "old" → "new"`} {
		if !strings.Contains(got, want) {
			t.Errorf("printDiff output missing %q:\n%s", want, got)
		}
	}
}

func TestUnrecordedRemovalsHintOnlyWhenSomeAreLeft(t *testing.T) {
	removes := []piholelists.Change{
		{Op: piholelists.Remove, Entry: piholelists.Entry{Kind: piholelists.Deny, Key: "a.example"}},
		{Op: piholelists.Remove, Entry: piholelists.Entry{Kind: piholelists.Deny, Key: "b.example"}},
	}
	if got := unrecordedRemovals(removes, removes); got != 0 {
		t.Errorf("every removal was applied, want 0, got %d", got)
	}
	if got := unrecordedRemovals(removes, removes[:1]); got != 1 {
		t.Errorf("one removal was left unchecked, want 1, got %d", got)
	}
	adds := []piholelists.Change{{Op: piholelists.Add, Entry: piholelists.Entry{Kind: piholelists.Deny, Key: "c.example"}}}
	if got := unrecordedRemovals(adds, nil); got != 0 {
		t.Errorf("an unapplied add is not an unrecorded entry, got %d", got)
	}
}

func TestCountByOp(t *testing.T) {
	added, updated, removed := countByOp([]piholelists.Change{
		{Op: piholelists.Add}, {Op: piholelists.Add}, {Op: piholelists.Update}, {Op: piholelists.Remove},
	})
	if added != 2 || updated != 1 || removed != 1 {
		t.Errorf("countByOp = %d/%d/%d, want 2/1/1", added, updated, removed)
	}
}

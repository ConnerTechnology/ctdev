package cmd

import (
	"testing"
)

func TestFindDuplicateAPTSources(t *testing.T) {
	files := map[string]string{
		"github-cli.list":     "deb [arch=amd64] https://cli.github.com/packages stable main\n",
		"github-cli-dup.list": "deb [arch=amd64] https://cli.github.com/packages stable main\n",
		"vscode.list":         "deb [arch=amd64] https://packages.microsoft.com/repos/code stable main\n",
	}
	dups := findDuplicateSourceLines(files)
	if len(dups) != 1 {
		t.Errorf("expected 1 duplicate, got %d", len(dups))
	}
}

func TestFindDuplicateAPTSourcesNoDups(t *testing.T) {
	files := map[string]string{
		"github-cli.list": "deb [arch=amd64] https://cli.github.com/packages stable main\n",
		"vscode.list":     "deb [arch=amd64] https://packages.microsoft.com/repos/code stable main\n",
	}
	dups := findDuplicateSourceLines(files)
	if len(dups) != 0 {
		t.Errorf("expected 0 duplicates, got %d", len(dups))
	}
}

func TestFindDuplicateAPTSourcesIgnoresComments(t *testing.T) {
	files := map[string]string{
		"a.list": "# comment\ndeb http://example.com stable main\n",
		"b.list": "# comment\ndeb http://other.com stable main\n",
	}
	dups := findDuplicateSourceLines(files)
	if len(dups) != 0 {
		t.Errorf("expected 0 duplicates (comments ignored), got %d", len(dups))
	}
}

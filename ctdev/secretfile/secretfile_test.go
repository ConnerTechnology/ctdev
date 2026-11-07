package secretfile

import (
	"os"
	"path/filepath"
	"testing"
)

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != want {
		t.Errorf("%s: mode %o, want %o", path, got, want)
	}
}

func TestParse(t *testing.T) {
	got := Parse([]byte("# comment\n\nexport A=one\nB = \"two\"\nC='three'\nnot a pair\nD=$(touch /tmp/x)\n"))
	want := map[string]string{"A": "one", "B": "two", "C": "three", "D": "$(touch /tmp/x)"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

func TestWritePrivateModes(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "creds")
	path := filepath.Join(dir, "x.env")
	if err := WritePrivate(path, []byte("A=1\n")); err != nil {
		t.Fatal(err)
	}
	assertMode(t, dir, 0o700)
	assertMode(t, path, 0o600)
}

func TestWriteAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.json")
	if err := WriteAtomic(path, []byte("one\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomic(path, []byte("two\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); string(data) != "two\n" {
		t.Errorf("content %q", data)
	}
	assertMode(t, path, 0o640)
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("temp files left behind: %d entries", len(entries))
	}
}

// Package secretfile reads and writes the KEY=VALUE files ctdev keeps a
// machine's credentials in, owner-only and never executed.
package secretfile

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strings"
)

// Parse reads KEY=VALUE lines. Blank lines and # comments are skipped, an
// `export ` prefix is tolerated, and one pair of matching quotes around a value
// is removed, so a hand-edited file still reads.
func Parse(data []byte) map[string]string {
	vals := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") || strings.HasPrefix(line, "export\t") {
			line = strings.TrimSpace(line[len("export"):])
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		val = strings.TrimSpace(val)
		if len(val) >= 2 && (val[0] == '"' || val[0] == '\'') && val[len(val)-1] == val[0] {
			val = val[1 : len(val)-1]
		}
		vals[key] = val
	}
	if sc.Err() != nil {
		return map[string]string{}
	}
	return vals
}

// WritePrivate writes a secret file atomically. The directory is made 0700
// and the file 0600.
func WritePrivate(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	return WriteAtomic(path, data, 0o600)
}

// WriteAtomic writes data to path through a temp file in the same directory
// and a rename, so a reader never sees half a file. The file gets mode; the
// directory must exist.
func WriteAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after a successful rename
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

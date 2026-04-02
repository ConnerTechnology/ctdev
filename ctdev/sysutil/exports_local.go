package sysutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func ExportsLocalPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".oh-my-zsh", "custom", "exports.local.zsh")
}

func SetLineInFile(path, key, newLine string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return os.WriteFile(path, []byte(newLine+"\n"), 0644)
		}
		return err
	}

	lines := strings.Split(string(content), "\n")
	found := false
	for i, line := range lines {
		if strings.Contains(line, key) && !strings.HasPrefix(strings.TrimSpace(line), "#") {
			lines[i] = newLine
			found = true
			break
		}
	}

	if !found {
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
			lines = append(lines, "")
		}
		lines = append(lines, newLine)
	}

	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
}

func AppendLineIfMissing(path, line string) (bool, error) {
	content, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}

	if strings.Contains(string(content), line) {
		return false, nil
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return false, err
	}
	defer f.Close()

	_, err = fmt.Fprintln(f, line)
	return err == nil, err
}

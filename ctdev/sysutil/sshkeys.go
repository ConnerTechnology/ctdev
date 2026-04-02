package sysutil

import (
	"os"
	"path/filepath"
	"strings"
)

type SSHPublicKey struct {
	Path    string
	Name    string
	KeyType string
}

func FindSSHPublicKeys() []SSHPublicKey {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return FindSSHPublicKeysIn(filepath.Join(home, ".ssh"))
}

func FindSSHPublicKeysIn(dir string) []SSHPublicKey {
	matches, err := filepath.Glob(filepath.Join(dir, "*.pub"))
	if err != nil {
		return nil
	}
	var keys []SSHPublicKey
	for _, path := range matches {
		name := filepath.Base(path)
		keyType := ""
		if data, err := os.ReadFile(path); err == nil {
			parts := strings.Fields(string(data))
			if len(parts) > 0 {
				keyType = parts[0]
			}
		}
		keys = append(keys, SSHPublicKey{
			Path:    path,
			Name:    name,
			KeyType: keyType,
		})
	}
	return keys
}

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

// SSHKeyPair is a private key with its public half beside it as <name>.pub.
type SSHKeyPair struct {
	Private string
	Public  string
	Type    string // the public key's algorithm, e.g. ssh-ed25519
	Comment string
}

// FindSSHKeyPairs lists the key pairs in ~/.ssh.
func FindSSHKeyPairs() []SSHKeyPair {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return FindSSHKeyPairsIn(filepath.Join(home, ".ssh"))
}

// FindSSHKeyPairsIn lists the key pairs in dir. A .pub with no private key
// beside it is left out: deleting it or adding it to ssh-agent wouldn't work.
func FindSSHKeyPairsIn(dir string) []SSHKeyPair {
	var pairs []SSHKeyPair
	for _, pub := range FindSSHPublicKeysIn(dir) {
		private := strings.TrimSuffix(pub.Path, ".pub")
		if fi, err := os.Stat(private); err != nil || !fi.Mode().IsRegular() {
			continue
		}
		pair := SSHKeyPair{Private: private, Public: pub.Path, Type: pub.KeyType}
		if data, err := os.ReadFile(pub.Path); err == nil {
			if fields := strings.Fields(string(data)); len(fields) > 2 {
				pair.Comment = strings.Join(fields[2:], " ")
			}
		}
		pairs = append(pairs, pair)
	}
	return pairs
}

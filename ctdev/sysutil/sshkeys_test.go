package sysutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindSSHPublicKeys(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "id_ed25519.pub"), []byte("ssh-ed25519 AAAA... user@host"), 0644)
	os.WriteFile(filepath.Join(dir, "id_rsa.pub"), []byte("ssh-rsa AAAA... user@host"), 0644)
	os.WriteFile(filepath.Join(dir, "id_ed25519"), []byte("private key"), 0600)
	os.WriteFile(filepath.Join(dir, "known_hosts"), []byte("host key"), 0644)

	keys := FindSSHPublicKeysIn(dir)
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}
}

func TestFindSSHPublicKeysEmpty(t *testing.T) {
	dir := t.TempDir()
	keys := FindSSHPublicKeysIn(dir)
	if len(keys) != 0 {
		t.Errorf("expected 0 keys, got %d", len(keys))
	}
}

func TestFindSSHPublicKeysKeyType(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "id_ed25519.pub"), []byte("ssh-ed25519 AAAA... user@host"), 0644)

	keys := FindSSHPublicKeysIn(dir)
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}
	if keys[0].KeyType != "ssh-ed25519" {
		t.Errorf("expected ssh-ed25519, got %s", keys[0].KeyType)
	}
}

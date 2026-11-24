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

func TestFindSSHKeyPairsNeedsBothHalves(t *testing.T) {
	dir := t.TempDir()
	write := func(name, data string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("id_ed25519", "private")
	write("id_ed25519.pub", "ssh-ed25519 AAAAexample test@example.invalid laptop")
	write("orphan.pub", "ssh-rsa AAAAexample test@example.invalid")
	write("known_hosts", "host key")

	pairs := FindSSHKeyPairsIn(dir)
	if len(pairs) != 1 {
		t.Fatalf("got %d pairs, want 1: %+v", len(pairs), pairs)
	}
	want := SSHKeyPair{
		Private: filepath.Join(dir, "id_ed25519"),
		Public:  filepath.Join(dir, "id_ed25519.pub"),
		Type:    "ssh-ed25519",
		Comment: "test@example.invalid laptop",
	}
	if pairs[0] != want {
		t.Errorf("got %+v, want %+v", pairs[0], want)
	}
}

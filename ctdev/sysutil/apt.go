package sysutil

import (
	"fmt"
	"os"
	"os/exec"
)

// AddAPTKeyring downloads a GPG key and installs it as an APT keyring.
func AddAPTKeyring(o Opts, url, keyringPath string) error {
	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] download GPG key %s → %s\n", url, keyringPath)
		return nil
	}
	tmp, err := os.CreateTemp("", "gpg-key-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	tmp.Close()

	if err := DownloadFile(url, tmp.Name()); err != nil {
		return fmt.Errorf("download GPG key: %w", err)
	}

	// Dearmor into the keyring path
	dearmor := exec.Command("gpg", "--dearmor", "--yes", "-o", keyringPath)
	in, err := os.Open(tmp.Name())
	if err != nil {
		return err
	}
	defer in.Close()
	dearmor.Stdin = in
	dearmor.Stdout = o.Stdout
	dearmor.Stderr = o.Stdout
	return dearmor.Run()
}

// AddAPTSource writes an APT sources list entry.
func AddAPTSource(o Opts, line, filename string) error {
	path := "/etc/apt/sources.list.d/" + filename
	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] write %s\n", path)
		return nil
	}
	return os.WriteFile(path, []byte(line+"\n"), 0644)
}

// APTUpdate runs apt-get update.
func APTUpdate(o Opts) error {
	return SudoRun(o, "apt-get", "update", "-qq")
}

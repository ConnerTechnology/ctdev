package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ConnerTechnology/dotfiles/ctdev/component"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

// Custom DNS (A records in dns.hosts) and CNAMEs (dns.cnameRecords) map internal
// names to private IPs, so they're stored SOPS-encrypted at
// configs/pihole/hosts/<hostname>.sops.json rather than as plaintext lists.

type customDNS struct {
	Hosts        []string `json:"hosts"`
	CnameRecords []string `json:"cnameRecords"`
}

func customDNSRelPath() string {
	host, _ := os.Hostname()
	if host == "" {
		host = "node"
	}
	return filepath.Join("hosts", host+".sops.json")
}

// exportCustomDNS reads Pi-hole's custom A/CNAME records and, when there are
// any, writes them SOPS-encrypted under dir/hosts/<hostname>.sops.json. Returns
// the number of records written (0 when there are none — nothing sensitive to
// store, so no file is created).
func exportCustomDNS(ctx context.Context, dir string) (int, error) {
	hosts := parsePiholeArray(piholeConfig(ctx, "dns.hosts"))
	cnames := parsePiholeArray(piholeConfig(ctx, "dns.cnameRecords"))
	if len(hosts) == 0 && len(cnames) == 0 {
		return 0, nil
	}
	if !sysutil.CommandExists("sops") {
		return 0, fmt.Errorf("sops is required to encrypt custom DNS records — install the 'sops' component")
	}

	payload, err := json.MarshalIndent(customDNS{Hosts: hosts, CnameRecords: cnames}, "", "  ")
	if err != nil {
		return 0, err
	}
	dst := filepath.Join(dir, customDNSRelPath())
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return 0, err
	}
	if err := os.WriteFile(dst, append(payload, '\n'), 0o600); err != nil {
		return 0, err
	}
	// Encrypt in place; .sops.yaml supplies the age recipient for this path.
	if out, err := exec.CommandContext(ctx, "sops", "--encrypt", "--in-place", dst).CombinedOutput(); err != nil {
		_ = os.Remove(dst) // don't leave plaintext behind on failure
		return 0, fmt.Errorf("sops encrypt: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return len(hosts) + len(cnames), nil
}

// importCustomDNS decrypts dir/hosts/<hostname>.sops.json (if present) and
// applies the records to Pi-hole, restarting FTL. A no-op when no file exists.
func importCustomDNS(ctx context.Context, o sysutil.Opts) error {
	enc, ok := readCustomDNSCipher()
	if !ok {
		return nil
	}
	if !sysutil.CommandExists("sops") {
		return fmt.Errorf("sops is required to decrypt custom DNS records — install the 'sops' component")
	}

	tmp, err := os.CreateTemp("", "pihole-dns-*.sops.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(enc); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()

	plain, err := exec.CommandContext(ctx, "sops", "--decrypt", tmp.Name()).Output()
	if err != nil {
		return fmt.Errorf("sops decrypt (is the age key at ~/.config/sops/age/keys.txt?): %w", err)
	}
	var dns customDNS
	if err := json.Unmarshal(plain, &dns); err != nil {
		return fmt.Errorf("parse decrypted custom DNS: %w", err)
	}

	if o.DryRun {
		fmt.Printf("  [dry-run] set %d custom DNS + %d CNAME records\n", len(dns.Hosts), len(dns.CnameRecords))
		return nil
	}
	if err := sysutil.PiholeRun(ctx, o, "pihole-FTL", "--config", "dns.hosts", jsonArray(dns.Hosts)); err != nil {
		return err
	}
	if err := sysutil.PiholeRun(ctx, o, "pihole-FTL", "--config", "dns.cnameRecords", jsonArray(dns.CnameRecords)); err != nil {
		return err
	}
	fmt.Printf("  custom DNS       %d records\n", len(dns.Hosts)+len(dns.CnameRecords))
	return sysutil.PiholeReload(ctx, o)
}

// readCustomDNSCipher returns the encrypted custom-DNS file from --from DIR when
// set, else from the embedded copy. ok is false when no file exists.
func readCustomDNSCipher() ([]byte, bool) {
	rel := customDNSRelPath()
	if flagPiholeFrom != "" {
		if b, err := os.ReadFile(filepath.Join(flagPiholeFrom, rel)); err == nil {
			return b, true
		}
		return nil, false
	}
	if b, err := component.Configs.ReadFile(piholeConfigSubdir + "/" + filepath.ToSlash(rel)); err == nil {
		return b, true
	}
	return nil, false
}

// piholeConfig reads a pihole-FTL config key from the container or host install.
func piholeConfig(ctx context.Context, key string) string {
	out, err := sysutil.PiholeCapture(ctx, "pihole-FTL", "--config", key)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// parsePiholeArray turns pihole-FTL's "[ a, b, c ]" array rendering into a slice
// (empty for "[]").
func parsePiholeArray(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// jsonArray renders a slice as a compact JSON array for `pihole-FTL --config`.
func jsonArray(items []string) string {
	b, _ := json.Marshal(items)
	if string(b) == "null" {
		return "[]"
	}
	return string(b)
}

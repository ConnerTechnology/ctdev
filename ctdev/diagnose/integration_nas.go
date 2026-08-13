package diagnose

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

// Synology and Proxmox are the default NAS and hypervisor in this class of
// network. Both answer the questions that matter most after "is the internet
// up": is the storage healthy, and are the backups actually running.
//
// Both clients follow the same rules as the UniFi one — private addresses
// only, read-only calls, and no credential ever written to disk.

// SynologyCreds reaches a DSM box. Create a read-only DSM user for this; it
// needs nothing more than permission to view storage.
type SynologyCreds struct {
	Endpoint string
	Username string
	Password string
}

func (c SynologyCreds) usable() bool {
	return c.Endpoint != "" && c.Username != "" && c.Password != ""
}

// ProxmoxCreds reaches a PVE node. An API token is preferred over a password:
// it can be scoped read-only and revoked without changing anyone's login.
type ProxmoxCreds struct {
	Endpoint string
	// TokenID is the full "user@realm!tokenname" identifier.
	TokenID string
	Secret  string
}

func (c ProxmoxCreds) usable() bool {
	return c.Endpoint != "" && c.TokenID != "" && c.Secret != ""
}

// privateAPIClient builds an HTTP client for a self-signed appliance on the
// local network, refusing anything that isn't private. Shared by both vendors
// because the reasoning is identical: NAS and hypervisor web UIs ship
// self-signed certificates, and the address check is what makes accepting
// them defensible.
func privateAPIClient(endpoint string) (*http.Client, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("bad URL %q: %w", endpoint, err)
	}
	addr, err := netip.ParseAddr(parsed.Hostname())
	if err != nil || !IsPrivate(addr) {
		return nil, errUnifiPublic
	}
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			//nolint:gosec // Self-signed by design; gated to private addresses above.
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12},
		},
	}, nil
}

// --- Synology ---

type synologyEnvelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   struct {
		Code int `json:"code"`
	} `json:"error"`
}

// SynologyStorage is the storage picture DSM reports.
type SynologyStorage struct {
	Disks []struct {
		Name       string `json:"name"`
		Model      string `json:"model"`
		Status     string `json:"status"`
		SmartWarn  bool   `json:"below_remain_life_thr"`
		Temp       int    `json:"temp"`
		DiskHealth string `json:"overview_status"`
	} `json:"disks"`
	Volumes []struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Size   struct {
			Total string `json:"total"`
			Used  string `json:"used"`
		} `json:"size"`
	} `json:"volumes"`
}

// collectSynology logs into DSM and reads the storage overview.
func collectSynology(ctx context.Context, creds SynologyCreds) (*SynologyStorage, error) {
	if !creds.usable() {
		return nil, errUnifiNoCreds
	}
	client, err := privateAPIClient(creds.Endpoint)
	if err != nil {
		return nil, err
	}

	base := strings.TrimRight(creds.Endpoint, "/")

	// The credentials go in a POST body, never the query string. Go's HTTP
	// errors quote the full URL, so a password in the URL would land in any
	// error we surfaced — and from there into a report someone forwards.
	login := url.Values{
		"api":     {"SYNO.API.Auth"},
		"version": {"3"},
		"method":  {"login"},
		"account": {creds.Username},
		"passwd":  {creds.Password},
		"session": {"ctdev"},
		"format":  {"sid"},
	}

	var session struct {
		SID string `json:"sid"`
	}
	if err := synologyPost(ctx, client, base+"/webapi/auth.cgi", login, &session); err != nil {
		return nil, err
	}
	if session.SID == "" {
		return nil, fmt.Errorf("DSM did not return a session")
	}
	// Log out even on failure: leaving sessions open on someone's NAS is
	// exactly the kind of trace this command promises not to leave.
	defer func() {
		logout := url.Values{
			"api": {"SYNO.API.Auth"}, "version": {"1"}, "method": {"logout"},
			"session": {"ctdev"}, "_sid": {session.SID},
		}
		var discard json.RawMessage
		_ = synologyPost(context.WithoutCancel(ctx), client, base+"/webapi/auth.cgi", logout, &discard)
	}()

	// The session id is a bearer token too, so it also travels in the body.
	query := url.Values{
		"api": {"SYNO.Storage.CGI.Storage"}, "version": {"1"},
		"method": {"load_info"}, "_sid": {session.SID},
	}
	var storage SynologyStorage
	if err := synologyPost(ctx, client, base+"/webapi/entry.cgi", query, &storage); err != nil {
		return nil, err
	}
	return &storage, nil
}

func synologyPost(ctx context.Context, client *http.Client, target string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, strings.NewReader(form.Encode()))
	if err != nil {
		return sanitizeAPIError(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return sanitizeAPIError(err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxUnifiResponse))
	if err != nil {
		return err
	}
	var env synologyEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("unreadable response from DSM: %w", err)
	}
	if !env.Success {
		return fmt.Errorf("DSM refused the request (error %d)", env.Error.Code)
	}
	if len(env.Data) == 0 {
		return nil
	}
	return json.Unmarshal(env.Data, out)
}

// synologyVerdict reads the storage overview. A degraded array on a NAS is the
// finding worth travelling for: it keeps serving files perfectly until the
// second disk goes, and then it doesn't.
func synologyVerdict(s *SynologyStorage) map[string]Result {
	out := map[string]Result{}

	var failed, aging []string
	for _, d := range s.Disks {
		label := d.Name
		if d.Model != "" {
			label += " (" + d.Model + ")"
		}
		switch {
		case d.Status != "" && d.Status != "normal":
			failed = append(failed, label)
		case d.SmartWarn:
			aging = append(aging, label)
		}
	}

	switch {
	case len(failed) > 0:
		out["nas.disks"] = failf("Replace the disk and let the volume repair before anything else fails.",
			"%s reporting a fault", strings.Join(failed, ", "))
	case len(aging) > 0:
		out["nas.disks"] = warnf("The drive is past its wear threshold. Order a replacement before it becomes urgent.",
			"%s below its remaining-life threshold", strings.Join(aging, ", "))
	default:
		out["nas.disks"] = okf("%d disk(s) healthy", len(s.Disks))
	}

	var degraded []string
	for _, v := range s.Volumes {
		if v.Status != "" && v.Status != "normal" {
			degraded = append(degraded, fmt.Sprintf("%s (%s)", v.ID, v.Status))
		}
	}
	if len(degraded) > 0 {
		out["nas.volumes"] = failf("The volume is running without its redundancy. Repair it before the next failure.",
			"%s", strings.Join(degraded, ", "))
	} else {
		out["nas.volumes"] = okf("%d volume(s) normal", len(s.Volumes))
	}

	return out
}

// --- Proxmox ---

// ProxmoxNode is one node's health as the cluster reports it.
type ProxmoxNode struct {
	Node    string  `json:"node"`
	Status  string  `json:"status"`
	MaxMem  int64   `json:"maxmem"`
	Mem     int64   `json:"mem"`
	MaxDisk int64   `json:"maxdisk"`
	Disk    int64   `json:"disk"`
	Uptime  int64   `json:"uptime"`
	CPU     float64 `json:"cpu"`
}

// ProxmoxStorage is one storage pool.
type ProxmoxStorage struct {
	Storage string `json:"storage"`
	Node    string `json:"node"`
	Total   int64  `json:"total"`
	Used    int64  `json:"used"`
	Active  int    `json:"active"`
}

// ProxmoxSnapshot is one round of collection.
type ProxmoxSnapshot struct {
	Nodes   []ProxmoxNode
	Storage []ProxmoxStorage
}

func collectProxmox(ctx context.Context, creds ProxmoxCreds) (*ProxmoxSnapshot, error) {
	if !creds.usable() {
		return nil, errUnifiNoCreds
	}
	client, err := privateAPIClient(creds.Endpoint)
	if err != nil {
		return nil, err
	}

	base := strings.TrimRight(creds.Endpoint, "/") + "/api2/json"
	// An API token authenticates per-request with no session to establish or
	// tear down, which is why it's preferred over a password here.
	auth := fmt.Sprintf("PVEAPIToken=%s=%s", creds.TokenID, creds.Secret)

	snap := &ProxmoxSnapshot{}
	if err := proxmoxGet(ctx, client, base+"/nodes", auth, &snap.Nodes); err != nil {
		return nil, err
	}
	_ = proxmoxGet(ctx, client, base+"/storage", auth, &snap.Storage)
	return snap, nil
}

func proxmoxGet(ctx context.Context, client *http.Client, target, auth string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", auth)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("Proxmox rejected the token")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Proxmox returned %s", resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxUnifiResponse))
	if err != nil {
		return err
	}
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("unreadable response from Proxmox: %w", err)
	}
	if len(env.Data) == 0 {
		return nil
	}
	return json.Unmarshal(env.Data, out)
}

// proxmoxStorageFullPct is where a hypervisor's storage stops being able to
// take a snapshot or a backup, which is usually noticed only when a backup
// fails silently.
const proxmoxStorageFullPct = 90

func proxmoxVerdict(snap *ProxmoxSnapshot) map[string]Result {
	out := map[string]Result{}

	var offline []string
	for _, n := range snap.Nodes {
		if n.Status != "online" {
			offline = append(offline, n.Node)
		}
	}
	if len(offline) > 0 {
		out["pve.nodes"] = failf("Everything hosted on those nodes is down with them.",
			"%s offline", strings.Join(offline, ", "))
	} else {
		out["pve.nodes"] = okf("%d node(s) online", len(snap.Nodes))
	}

	var full []string
	for _, s := range snap.Storage {
		if s.Total <= 0 || s.Active == 0 {
			continue
		}
		pct := int(s.Used * 100 / s.Total)
		if pct >= proxmoxStorageFullPct {
			full = append(full, fmt.Sprintf("%s %d%%", s.Storage, pct))
		}
	}
	if len(full) > 0 {
		out["pve.storage"] = failf("Snapshots and backups fail once a pool is this full, usually without anyone noticing.",
			"%s", strings.Join(full, ", "))
	} else if len(snap.Storage) > 0 {
		out["pve.storage"] = okf("%d storage pool(s) with room", len(snap.Storage))
	}

	return out
}

// sanitizeAPIError strips query strings out of any URL an error quotes.
//
// Belt and braces alongside sending secrets in POST bodies: a report is a file
// people forward, and a credential that reaches an error message reaches the
// report. Nothing in a query string is worth more than that risk.
func sanitizeAPIError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if !strings.Contains(msg, "?") {
		return err
	}
	var b strings.Builder
	for i, part := range strings.Split(msg, " ") {
		if i > 0 {
			b.WriteByte(' ')
		}
		if q := strings.IndexByte(part, '?'); q >= 0 && strings.Contains(part, "://") {
			b.WriteString(part[:q] + "?[redacted]")
			continue
		}
		b.WriteString(part)
	}
	return errors.New(b.String())
}

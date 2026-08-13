package diagnose

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The same guarantee the UniFi client makes: a credential never leaves for a
// public address over a connection we can't verify.
func TestPrivateAPIClientRefusesPublic(t *testing.T) {
	for _, endpoint := range []string{"https://203.0.113.9", "https://8.8.8.8:5001"} {
		if _, err := privateAPIClient(endpoint); !errors.Is(err, errUnifiPublic) {
			t.Errorf("%s: expected a refusal, got %v", endpoint, err)
		}
	}
	if _, err := privateAPIClient("https://192.168.1.50:5001"); err != nil {
		t.Errorf("private address should be accepted, got %v", err)
	}
}

// synologyFixture builds a healthy storage picture. It returns a fresh value
// each call so a mutated case can't leak into the next one.
func synologyFixture() *SynologyStorage {
	s := &SynologyStorage{}
	s.Disks = append(s.Disks, struct {
		Name       string `json:"name"`
		Model      string `json:"model"`
		Status     string `json:"status"`
		SmartWarn  bool   `json:"below_remain_life_thr"`
		Temp       int    `json:"temp"`
		DiskHealth string `json:"overview_status"`
	}{Name: "Drive 1", Model: "WD40EFRX", Status: "normal"})
	s.Volumes = append(s.Volumes, struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Size   struct {
			Total string `json:"total"`
			Used  string `json:"used"`
		} `json:"size"`
	}{ID: "volume_1", Status: "normal"})
	return s
}

func TestSynologyVerdict(t *testing.T) {
	healthy := synologyFixture()
	res := synologyVerdict(healthy)
	if res["nas.disks"].Severity != OK || res["nas.volumes"].Severity != OK {
		t.Errorf("healthy NAS = %+v", res)
	}

	// Each case gets its own storage: copying the struct would share the
	// underlying slice array and carry the previous mutation forward.
	degraded := synologyFixture()
	degraded.Volumes[0].Status = "degraded"
	if got := synologyVerdict(degraded)["nas.volumes"].Severity; got != Fail {
		t.Errorf("degraded volume = %v, want Fail", got)
	}

	failing := synologyFixture()
	failing.Disks[0].Status = "crashed"
	if got := synologyVerdict(failing)["nas.disks"].Severity; got != Fail {
		t.Errorf("crashed disk = %v, want Fail", got)
	}

	worn := synologyFixture()
	worn.Disks[0].SmartWarn = true
	if got := synologyVerdict(worn)["nas.disks"].Severity; got != Warn {
		t.Errorf("worn disk = %v, want Warn", got)
	}
}

func TestProxmoxVerdict(t *testing.T) {
	healthy := &ProxmoxSnapshot{
		Nodes:   []ProxmoxNode{{Node: "pve1", Status: "online"}},
		Storage: []ProxmoxStorage{{Storage: "local-zfs", Total: 1000, Used: 400, Active: 1}},
	}
	res := proxmoxVerdict(healthy)
	if res["pve.nodes"].Severity != OK || res["pve.storage"].Severity != OK {
		t.Errorf("healthy cluster = %+v", res)
	}

	offline := &ProxmoxSnapshot{Nodes: []ProxmoxNode{{Node: "pve2", Status: "offline"}}}
	if got := proxmoxVerdict(offline)["pve.nodes"]; got.Severity != Fail || !strings.Contains(got.Detail, "pve2") {
		t.Errorf("offline node = %v %q", got.Severity, got.Detail)
	}

	// Snapshots and backups fail silently once a pool is this full.
	full := &ProxmoxSnapshot{
		Nodes:   []ProxmoxNode{{Node: "pve1", Status: "online"}},
		Storage: []ProxmoxStorage{{Storage: "local-zfs", Total: 1000, Used: 950, Active: 1}},
	}
	if got := proxmoxVerdict(full)["pve.storage"]; got.Severity != Fail {
		t.Errorf("full storage = %v, want Fail", got.Severity)
	}

	// An inactive pool has no meaningful usage and must not be reported.
	inactive := &ProxmoxSnapshot{
		Nodes:   []ProxmoxNode{{Node: "pve1", Status: "online"}},
		Storage: []ProxmoxStorage{{Storage: "unused", Total: 1000, Used: 1000, Active: 0}},
	}
	if got := proxmoxVerdict(inactive)["pve.storage"]; got.Severity == Fail {
		t.Error("an inactive storage pool should not be reported as full")
	}
}

// DSM sessions are server-side state. Leaving them open on someone's NAS is
// exactly the trace this command promises not to leave.
func TestSynologyLogsOut(t *testing.T) {
	var sawLogout bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Credentials travel in the body now, so the method does too.
		_ = r.ParseForm()
		if r.URL.RawQuery != "" {
			t.Errorf("request carried a query string, which can leak into errors: %q", r.URL.RawQuery)
		}
		switch r.PostFormValue("method") {
		case "login":
			_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"abc"}}`))
		case "logout":
			sawLogout = true
			_, _ = w.Write([]byte(`{"success":true}`))
		default:
			_, _ = w.Write([]byte(`{"success":true,"data":{"disks":[],"volumes":[]}}`))
		}
	}))
	defer srv.Close()

	if _, err := collectSynology(context.Background(), SynologyCreds{
		Endpoint: srv.URL, Username: "readonly", Password: "pw",
	}); err != nil {
		t.Fatalf("collectSynology: %v", err)
	}
	if !sawLogout {
		t.Error("the DSM session was left open")
	}
}

func TestProxmoxSendsTokenAuth(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"data":[{"node":"pve1","status":"online"}]}`))
	}))
	defer srv.Close()

	snap, err := collectProxmox(context.Background(), ProxmoxCreds{
		Endpoint: srv.URL, TokenID: "ctdev@pve!diag", Secret: "s3cr3t",
	})
	if err != nil {
		t.Fatalf("collectProxmox: %v", err)
	}
	if want := "PVEAPIToken=ctdev@pve!diag=s3cr3t"; gotAuth != want {
		t.Errorf("Authorization = %q, want %q", gotAuth, want)
	}
	if len(snap.Nodes) != 1 {
		t.Errorf("nodes = %+v", snap.Nodes)
	}
}

// Every vendor secret must be unreachable from a rendered report.
func TestNASCredentialsNeverReachTheReport(t *testing.T) {
	const secret = "nas-password-do-not-print"
	checks, results := CollectIntegrations(context.Background(), Integrations{
		Synology: SynologyCreds{Endpoint: unreachableController, Username: "u", Password: secret},
		Proxmox:  ProxmoxCreds{Endpoint: unreachableController, TokenID: "t", Secret: secret},
	})

	r := Report{Checks: checks, Results: results, Facts: Facts{Hostname: "h"}}
	if strings.Contains(Markdown(r), secret) {
		t.Fatal("a credential reached the Markdown report")
	}
	if strings.Contains(Render(r, 100), secret) {
		t.Fatal("a credential reached the terminal report")
	}
}

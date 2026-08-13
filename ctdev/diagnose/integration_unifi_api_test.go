package diagnose

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// unreachableController is a loopback port nothing listens on: private (so the
// address guard lets it through) and refused instantly (so the test doesn't sit
// out a connection timeout).
const unreachableController = "https://127.0.0.1:1"

// Sending an API key to a public address over a connection we can't verify
// would hand the credential to whoever is in the middle. This is the guarantee
// that makes skipping certificate verification defensible at all.
func TestUnifiRefusesPublicEndpoints(t *testing.T) {
	for _, endpoint := range []string{
		"https://203.0.113.9",
		"https://8.8.8.8:8443",
		"https://unifi.example.com",
	} {
		_, err := newUnifiClient(UnifiCreds{Endpoint: endpoint, APIKey: "secret"})
		if err == nil {
			t.Errorf("%s: expected a refusal, got a usable client", endpoint)
			continue
		}
		if !errors.Is(err, errUnifiPublic) && !strings.Contains(err.Error(), "resolve") {
			t.Errorf("%s: refused for the wrong reason: %v", endpoint, err)
		}
	}
}

func TestUnifiAcceptsPrivateEndpoints(t *testing.T) {
	for _, endpoint := range []string{
		"https://192.168.1.1",
		"https://10.2.2.1:8443",
		"https://172.16.0.1",
		// Carrier-grade NAT is where an ISP-supplied router often sits.
		"https://100.64.0.1",
	} {
		if _, err := newUnifiClient(UnifiCreds{Endpoint: endpoint, APIKey: "k"}); err != nil {
			t.Errorf("%s: expected acceptance, got %v", endpoint, err)
		}
	}
}

func TestUnifiRequiresCredentials(t *testing.T) {
	_, err := newUnifiClient(UnifiCreds{Endpoint: "https://192.168.1.1"})
	if !errors.Is(err, errUnifiNoCreds) {
		t.Errorf("expected errUnifiNoCreds, got %v", err)
	}
}

// The two controller generations put the same data at different paths, and a
// single API key reaches both.
func TestUnifiAPIPath(t *testing.T) {
	modern, err := newUnifiClient(UnifiCreds{Endpoint: "https://192.168.1.1", APIKey: "k"})
	if err != nil {
		t.Fatal(err)
	}
	if got := modern.apiPath("/stat/device"); got != "https://192.168.1.1/proxy/network/api/s/default/stat/device" {
		t.Errorf("UniFi OS path = %q", got)
	}

	legacy, err := newUnifiClient(UnifiCreds{Endpoint: "https://192.168.1.1:8443", APIKey: "k", Site: "office"})
	if err != nil {
		t.Fatal(err)
	}
	if got := legacy.apiPath("/stat/device"); got != "https://192.168.1.1:8443/api/s/office/stat/device" {
		t.Errorf("legacy path = %q", got)
	}
}

// newTestClient points a client at an httptest server while keeping the rest of
// the client's behaviour intact. The private-address guard is satisfied because
// httptest listens on loopback.
func newTestClient(t *testing.T, handler http.Handler) (*unifiClient, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	client, err := newUnifiClient(UnifiCreds{Endpoint: srv.URL, APIKey: "test-key"})
	if err != nil {
		t.Fatalf("newUnifiClient: %v", err)
	}
	// Use the server's own client so plain HTTP works in tests.
	client.http = srv.Client()
	return client, srv
}

func TestUnifiGetSendsAPIKey(t *testing.T) {
	var gotKey string
	client, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-API-KEY")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"mac":"aa:bb","name":"AP"}]}`))
	}))

	var devices []UnifiDevice
	if err := client.get(context.Background(), "/stat/device", &devices); err != nil {
		t.Fatalf("get: %v", err)
	}
	if gotKey != "test-key" {
		t.Errorf("X-API-KEY = %q, want the configured key", gotKey)
	}
	if len(devices) != 1 || devices[0].Name != "AP" {
		t.Errorf("devices = %+v", devices)
	}
}

func TestUnifiGetErrorPaths(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantMsg string
	}{
		{"rejected credentials", http.StatusUnauthorized, ``, "rejected the credentials"},
		{"rate limited", http.StatusTooManyRequests, ``, "rate-limited"},
		{"server error", http.StatusInternalServerError, ``, "returned"},
		{"api-level error", http.StatusOK, `{"meta":{"rc":"error","msg":"api.err.NoSiteContext"},"data":[]}`, "api.err.NoSiteContext"},
		{"unparseable body", http.StatusOK, `<html>login</html>`, "unreadable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			var out []UnifiDevice
			err := client.get(context.Background(), "/stat/device", &out)
			if err == nil {
				t.Fatalf("expected an error")
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error = %q, want it to mention %q", err, tt.wantMsg)
			}
		})
	}
}

// The controller is not ours and may hang. A stalled probe must not stall the
// report — the context has to carry through.
func TestUnifiGetHonorsContext(t *testing.T) {
	client, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var out []UnifiDevice
	if err := client.get(ctx, "/stat/device", &out); err == nil {
		t.Error("expected the cancelled context to abort the request")
	}
}

// A legacy controller authenticates by session cookie. Without a jar the
// cookie is dropped and every later request goes out unauthenticated, which
// looks exactly like bad credentials.
func TestUnifiLegacyLoginKeepsSession(t *testing.T) {
	var sawCookie bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/api/login") {
			http.SetCookie(w, &http.Cookie{Name: "unifises", Value: "abc123", Path: "/"})
			w.Header().Set("X-CSRF-Token", "csrf-1")
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[]}`))
			return
		}
		if c, err := r.Cookie("unifises"); err == nil && c.Value == "abc123" {
			sawCookie = true
		}
		if r.Header.Get("X-CSRF-Token") != "csrf-1" {
			t.Errorf("CSRF token was not echoed on the follow-up request")
		}
		_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[]}`))
	}))
	defer srv.Close()

	client, err := newUnifiClient(UnifiCreds{Endpoint: srv.URL, Username: "admin", Password: "pw"})
	if err != nil {
		t.Fatal(err)
	}
	client.http.Transport = srv.Client().Transport
	// httptest serves plain HTTP on a loopback port, so force the legacy path.
	client.legacy = true

	if err := client.login(context.Background()); err != nil {
		t.Fatalf("login: %v", err)
	}
	var out []UnifiDevice
	if err := client.get(context.Background(), "/stat/device", &out); err != nil {
		t.Fatalf("get after login: %v", err)
	}
	if !sawCookie {
		t.Error("the session cookie was not carried to the next request")
	}
}

// Losing the device list is fatal to the snapshot, but the enrichment calls
// are not: a controller that withholds one of them shouldn't cost us the rest.
func TestCollectUnifiToleratesPartialFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/stat/device"):
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"mac":"aa","name":"AP","state":1,"type":"uap"}]}`))
		default:
			w.WriteHeader(http.StatusForbidden)
		}
	}))
	defer srv.Close()

	snap, err := collectUnifi(context.Background(), UnifiCreds{Endpoint: srv.URL, APIKey: "k"})
	if err != nil {
		t.Fatalf("collectUnifi: %v", err)
	}
	if len(snap.Devices) != 1 {
		t.Errorf("devices = %+v, want the one the controller did return", snap.Devices)
	}
	if len(snap.Events) != 0 || len(snap.WLANs) != 0 {
		t.Error("expected the forbidden enrichment calls to yield nothing, not fail the run")
	}
}

// CollectIntegrations must make no outbound call at all without credentials.
func TestCollectIntegrationsNoCredentialsIsSilent(t *testing.T) {
	checks, results := CollectIntegrations(context.Background(), Integrations{})
	if len(checks) != 0 || len(results) != 0 {
		t.Errorf("expected no vendor checks without credentials, got %d/%d", len(checks), len(results))
	}
}

// A controller that refuses us is one honest row, not a failed run: the local
// diagnosis stands on its own.
func TestCollectIntegrationsReportsAccessFailure(t *testing.T) {
	checks, results := CollectIntegrations(context.Background(), Integrations{
		UniFi: UnifiCreds{Endpoint: unreachableController, APIKey: "k"},
	})
	if len(checks) != 1 || checks[0].ID != "unifi.access" {
		t.Fatalf("expected a single access row, got %+v", checks)
	}
	if results["unifi.access"].Severity != Skipped {
		t.Errorf("severity = %v, want Skipped", results["unifi.access"].Severity)
	}
}

// The API key must never reach a rendered report, whatever else goes in Data.
func TestUnifiCredentialsNeverReachTheReport(t *testing.T) {
	const key = "unifi-api-key-do-not-print"
	checks, results := CollectIntegrations(context.Background(), Integrations{
		UniFi: UnifiCreds{Endpoint: unreachableController, APIKey: key},
	})

	r := Report{Checks: checks, Results: results, Facts: Facts{Hostname: "h"}}
	if strings.Contains(Markdown(r), key) {
		t.Fatal("the API key reached the Markdown report")
	}
	if strings.Contains(Render(r, 100), key) {
		t.Fatal("the API key reached the terminal report")
	}
}

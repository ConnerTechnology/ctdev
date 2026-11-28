package diagnose

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

// unreachableController is a loopback port nothing listens on: private (so the
// address guard lets it through) and refused instantly (so the test doesn't sit
// out a connection timeout).
const unreachableController = "https://127.0.0.1:1"

// Every numeric address actually dialed must be private, even for a hostname
// whose answer contains both LAN and non-LAN addresses.
func TestUnifiRefusesPublicEndpoints(t *testing.T) {
	for _, endpoint := range []string{"https://203.0.113.9", "https://8.8.8.8:8443"} {
		_, err := newUnifiClient(UnifiCreds{Endpoint: endpoint, APIKey: "example"})
		if !errors.Is(err, errPublicAppliance) {
			t.Errorf("%s: got %v", endpoint, err)
		}
	}
	original := lookupHostIP
	lookupHostIP = func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("127.0.0.1"), netip.MustParseAddr("203.0.113.9")}, nil
	}
	t.Cleanup(func() { lookupHostIP = original })
	client, err := newUnifiClient(UnifiCreds{Endpoint: "https://controller.example.invalid", APIKey: "example"})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.get(t.Context(), "/stat/device", nil); !errors.Is(err, errPublicAppliance) {
		t.Fatalf("got %v, want private-address rejection", err)
	}
}

func certPin(srv *httptest.Server) string {
	return fmt.Sprintf("%x", sha256.Sum256(srv.Certificate().Raw))
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
	srv := httptest.NewTLSServer(handler)
	t.Cleanup(srv.Close)

	client, err := newUnifiClient(UnifiCreds{Endpoint: srv.URL, CertSHA256: certPin(srv), APIKey: "test-key"})
	if err != nil {
		t.Fatalf("newUnifiClient: %v", err)
	}
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
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	client, err := newUnifiClient(UnifiCreds{Endpoint: srv.URL, CertSHA256: certPin(srv), Username: "admin", Password: "pw"})
	if err != nil {
		t.Fatal(err)
	}
	// The random HTTPS test port needs the legacy controller path.
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
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/stat/device"):
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"mac":"aa","name":"AP","state":1,"type":"uap"}]}`))
		default:
			w.WriteHeader(http.StatusForbidden)
		}
	}))
	defer srv.Close()

	snap, err := collectUnifi(context.Background(), UnifiCreds{Endpoint: srv.URL, CertSHA256: certPin(srv), APIKey: "k"})
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

// The API key must never reach a rendered report, whether the controller is
// unreachable or echoes back an HTTP error containing it.
func TestUnifiCredentialsNeverReachTheReport(t *testing.T) {
	const key = "unifi-api-key-do-not-print"
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("invalid X-API-KEY: " + key))
	}))
	defer srv.Close()

	for name, endpoint := range map[string]string{
		"unreachable": unreachableController,
		"http 401":    srv.URL,
	} {
		t.Run(name, func(t *testing.T) {
			checks, results := CollectIntegrations(context.Background(), Integrations{
				UniFi: UnifiCreds{Endpoint: endpoint, CertSHA256: certPin(srv), APIKey: key},
			})
			r := Report{Checks: checks, Results: results, Facts: Facts{Hostname: "h"}}
			if strings.Contains(Markdown(r), key) {
				t.Fatal("the API key reached the Markdown report")
			}
			if strings.Contains(Render(r, 100), key) {
				t.Fatal("the API key reached the terminal report")
			}
		})
	}
}

func TestApplianceHTTPSAuthenticatesBeforeRequests(t *testing.T) {
	requests := make(chan struct{}, 1)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests <- struct{}{}
	}))
	defer srv.Close()
	for _, tc := range []struct{ name, endpoint, pin string }{
		{"HTTP", strings.Replace(srv.URL, "https:", "http:", 1), ""},
		{"untrusted certificate", srv.URL, ""},
		{"different certificate", srv.URL, strings.Repeat("ab", 32)},
		{"truncated pin", srv.URL, certPin(srv)[:32]},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, err := privateAPIClient(tc.endpoint, tc.pin)
			if err == nil {
				resp, reqErr := client.Get(tc.endpoint)
				if resp != nil {
					resp.Body.Close()
				}
				err = reqErr
			}
			if err == nil {
				t.Fatal("expected authentication to fail")
			}
			select {
			case <-requests:
				t.Fatal("sent an HTTP request")
			default:
			}
		})
	}
	client, err := privateAPIClient(srv.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	// Configure the test server's CA as a trusted root for this client.
	client.Transport.(*http.Transport).TLSClientConfig.RootCAs = srv.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	<-requests
	req, err := http.NewRequest(http.MethodGet, strings.Replace(srv.URL, "https:", "http:", 1), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.CheckRedirect(req, nil); err == nil {
		t.Fatal("accepted a same-host downgrade")
	}
}

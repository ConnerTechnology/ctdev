package diagnose

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
)

// UnifiCreds is how to reach a controller. It is passed in and never persisted
// by this package — where it came from is the command layer's problem.
type UnifiCreds struct {
	// Endpoint is the controller base URL, e.g. https://192.168.1.1.
	Endpoint string
	// CertSHA256 optionally pins the complete DER-encoded leaf certificate.
	CertSHA256 string
	// APIKey authenticates against UniFi OS consoles (Settings → Control
	// Plane → Integrations).
	APIKey string
	// Username and Password authenticate against legacy self-hosted
	// controllers on :8443, which predate API keys.
	Username string
	Password string
	// Site is the UniFi site identifier. Almost always "default".
	Site string
}

func (c UnifiCreds) usable() bool {
	return c.Endpoint != "" && (c.APIKey != "" || (c.Username != "" && c.Password != ""))
}

func (c UnifiCreds) site() string {
	if c.Site == "" {
		return "default"
	}
	return c.Site
}

var errUnifiNoCreds = errors.New("no controller credentials")

// unifiClient talks to one controller, read-only.
type unifiClient struct {
	creds UnifiCreds
	http  *http.Client
	// legacy marks a self-hosted controller, whose API lives at a different
	// path and authenticates with a session cookie.
	legacy bool
	csrf   string

	CertFingerprint string
}

func newUnifiClient(creds UnifiCreds) (*unifiClient, error) {
	if !creds.usable() {
		return nil, errUnifiNoCreds
	}
	client, err := privateAPIClient(creds.Endpoint, creds.CertSHA256)
	if err != nil {
		return nil, err
	}
	// A legacy controller authenticates subsequent requests by session cookie.
	client.Jar, err = cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	origin, _ := url.Parse(creds.Endpoint) // validated by privateAPIClient
	return &unifiClient{creds: creds, http: client, legacy: origin.Port() == "8443"}, nil
}

// apiPath builds the request path for whichever controller generation this is.
//
// The official integration API is cleaner but far thinner; the diagnostic
// detail — radar events, airtime, per-client signal — only exists under the
// classic paths, which the same API key reaches.
func (inst *unifiClient) apiPath(suffix string) string {
	base := strings.TrimRight(inst.creds.Endpoint, "/")
	if inst.legacy {
		return fmt.Sprintf("%s/api/s/%s%s", base, inst.creds.site(), suffix)
	}
	return fmt.Sprintf("%s/proxy/network/api/s/%s%s", base, inst.creds.site(), suffix)
}

// login establishes a session on a legacy controller. UniFi OS consoles use a
// stateless header instead and need nothing here.
func (inst *unifiClient) login(ctx context.Context) error {
	if inst.creds.APIKey != "" {
		return nil
	}

	base := strings.TrimRight(inst.creds.Endpoint, "/")
	path := base + "/api/login"
	if !inst.legacy {
		path = base + "/api/auth/login"
	}

	body, err := json.Marshal(map[string]string{
		"username": inst.creds.Username,
		"password": inst.creds.Password,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, path, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := inst.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("login rejected (%s)", resp.Status)
	}

	// UniFi OS hands back a CSRF token that later requests must echo.
	if token := resp.Header.Get("X-CSRF-Token"); token != "" {
		inst.csrf = token
	}
	return nil
}

// unifiEnvelope is the classic API's response wrapper.
type unifiEnvelope struct {
	Meta struct {
		RC  string `json:"rc"`
		Msg string `json:"msg"`
	} `json:"meta"`
	Data json.RawMessage `json:"data"`
}

// maxUnifiResponse bounds how much we'll read from a controller. A busy site's
// event list is large, and this is a diagnostic, not an archive.
const maxUnifiResponse = 8 << 20

// get fetches and unwraps a classic-API endpoint into out.
func (inst *unifiClient) get(ctx context.Context, suffix string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, inst.apiPath(suffix), nil)
	if err != nil {
		return err
	}
	if inst.creds.APIKey != "" {
		req.Header.Set("X-API-KEY", inst.creds.APIKey)
	}
	if inst.csrf != "" {
		req.Header.Set("X-CSRF-Token", inst.csrf)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := inst.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	fingerprint := sha256.Sum256(resp.TLS.PeerCertificates[0].Raw)
	inst.CertFingerprint = fmt.Sprintf("%x", fingerprint)

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("controller rejected the credentials (%s)", resp.Status)
	case http.StatusTooManyRequests:
		return fmt.Errorf("controller rate-limited the request (retry after %s)", resp.Header.Get("Retry-After"))
	default:
		return fmt.Errorf("controller returned %s", resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxUnifiResponse))
	if err != nil {
		return err
	}

	var env unifiEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("unreadable response from the controller: %w", err)
	}
	if env.Meta.RC != "" && env.Meta.RC != "ok" {
		return fmt.Errorf("controller error: %s", env.Meta.Msg)
	}
	if len(env.Data) == 0 {
		return nil
	}
	return json.Unmarshal(env.Data, out)
}

// --- response shapes, trimmed to the fields the diagnosis actually uses ---

// UnifiDevice is one adopted device: a gateway, switch, or access point.
type UnifiDevice struct {
	MAC     string `json:"mac"`
	Name    string `json:"name"`
	Model   string `json:"model"`
	Type    string `json:"type"`
	State   int    `json:"state"`
	Adopted bool   `json:"adopted"`
	Version string `json:"version"`
	NumSta  int    `json:"num_sta"`
	Uplink  struct {
		Type string `json:"type"`
	} `json:"uplink"`
	RadioStats []UnifiRadioStat `json:"radio_table_stats"`
}

// UnifiRadioStat is one radio's airtime picture. cu_total is the share of the
// channel that is busy — the number that explains "the Wi-Fi is slow" when
// every client shows a strong signal.
type UnifiRadioStat struct {
	Name         string `json:"name"`
	Radio        string `json:"radio"`
	Channel      int    `json:"channel"`
	CuTotal      int    `json:"cu_total"`
	CuSelfRx     int    `json:"cu_self_rx"`
	CuSelfTx     int    `json:"cu_self_tx"`
	Satisfaction int    `json:"satisfaction"`
	NumSta       int    `json:"num_sta"`
}

// UnifiEvent is one controller event.
type UnifiEvent struct {
	Key  string `json:"key"`
	Msg  string `json:"msg"`
	Time int64  `json:"time"`
	AP   string `json:"ap"`
}

// UnifiHealth is one subsystem's status.
type UnifiHealth struct {
	Subsystem       string `json:"subsystem"`
	Status          string `json:"status"`
	NumAP           int    `json:"num_ap"`
	NumDisconnected int    `json:"num_disconnected"`
	WANIP           string `json:"wan_ip"`
}

// UnifiWLAN is one configured wireless network.
type UnifiWLAN struct {
	Name           string `json:"name"`
	Enabled        bool   `json:"enabled"`
	MinRSSIEnabled bool   `json:"minrssi_enabled"`
	MinRSSI        int    `json:"minrssi"`
}

// UnifiSnapshot is everything one round of collection gathered.
type UnifiSnapshot struct {
	Devices []UnifiDevice
	Events  []UnifiEvent
	Health  []UnifiHealth
	WLANs   []UnifiWLAN

	CertPinned      bool
	CertFingerprint string
}

// collectUnifi gathers the read-only picture from a controller. Every call is
// a GET; nothing here can change a setting.
func collectUnifi(ctx context.Context, creds UnifiCreds) (*UnifiSnapshot, error) {
	client, err := newUnifiClient(creds)
	if err != nil {
		return nil, err
	}
	if err := client.login(ctx); err != nil {
		return nil, err
	}

	snap := &UnifiSnapshot{}

	// The device list is the one that must succeed; the rest are enrichment,
	// and a controller that withholds one of them shouldn't cost us the lot.
	if err := client.get(ctx, "/stat/device", &snap.Devices); err != nil {
		return nil, err
	}
	snap.CertPinned = creds.CertSHA256 != ""
	snap.CertFingerprint = client.CertFingerprint

	_ = client.get(ctx, "/stat/health", &snap.Health)
	_ = client.get(ctx, "/rest/wlanconf", &snap.WLANs)
	// Events are capped by the controller; the recent ones are what matter.
	_ = client.get(ctx, "/stat/event?_limit=200&within=24", &snap.Events)

	return snap, nil
}

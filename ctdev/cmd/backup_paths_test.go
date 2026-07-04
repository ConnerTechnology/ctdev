package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExcludeMatches(t *testing.T) {
	tests := []struct {
		pattern, path string
		want          bool
	}{
		{"/home/x/Downloads", "/home/x/Downloads", true},
		{"/home/x/Downloads", "/home/x/Downloads/big.iso", true},
		{"/home/x/Downloads", "/home/x/Downloadsfoo", false}, // prefix must be a path boundary
		{"*.iso", "/home/x/movie.iso", true},
		{"*.iso", "/home/x/movie.mp4", false},
		{"**/node_modules", "/home/x/proj/node_modules", true},
		{"**/node_modules", "/home/x/proj/node_modules/pkg", true},
		{"**/node_modules", "/home/x/proj/src", false},
	}
	for _, tt := range tests {
		if got := excludeMatches(tt.pattern, tt.path); got != tt.want {
			t.Errorf("excludeMatches(%q,%q)=%v want %v", tt.pattern, tt.path, got, tt.want)
		}
	}
}

func TestEntryState(t *testing.T) {
	inc := map[string]bool{"/home/x": true}
	exc := []string{"/home/x/Downloads", "*.iso"}
	cases := map[string]string{
		"/home/x":           "included",
		"/home/x/Repos":     "inherited",
		"/home/x/Downloads": "excluded",
		"/home/x/a.iso":     "excluded",
		"/var/other":        "neutral",
	}
	for path, want := range cases {
		if got := entryState(path, inc, exc); got != want {
			t.Errorf("entryState(%q)=%q want %q", path, got, want)
		}
	}
}

func TestListDir(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "sub"), 0o755)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("hi"), 0o644)
	os.WriteFile(filepath.Join(dir, ".hidden"), []byte("x"), 0o644)

	entries, err := listDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("want 3 entries, got %d", len(entries))
	}
	// Directory sorts first.
	if !entries[0].isDir || entries[0].name != "sub" {
		t.Errorf("expected dir 'sub' first, got %+v", entries[0])
	}
	// A regular file reports its byte size.
	var btxt *dirEntry
	for i := range entries {
		if entries[i].name == "b.txt" {
			btxt = &entries[i]
		}
	}
	if btxt == nil || btxt.size != 2 {
		t.Errorf("b.txt size wrong: %+v", btxt)
	}
}

func TestCreatedUnix(t *testing.T) {
	f := filepath.Join(t.TempDir(), "x")
	os.WriteFile(f, []byte("hi"), 0o644)
	fi, err := os.Lstat(f)
	if err != nil {
		t.Fatal(err)
	}
	got := createdUnix(f, fi)
	// A just-created file's birth (or fallback mtime) is recent and non-zero.
	if got <= 0 {
		t.Errorf("createdUnix returned %d, want > 0", got)
	}
}

func newTestPicker() *pathPicker {
	return &pathPicker{
		bootToken: "boot-secret",
		apiToken:  "secret",
		home:      "/home/x",
		includes:  map[string]bool{"/home/x": true},
		excludes:  []string{"*.iso"},
		sizeCache: map[string]int64{},
		done:      make(chan struct{}),
	}
}

// withSession attaches the session cookie the boot-token exchange would issue.
func withSession(req *http.Request) *http.Request {
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: "secret"})
	return req
}

func TestGuardRejectsBadSession(t *testing.T) {
	p := newTestPicker()
	h := p.routes()

	// No cookie → 403.
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/api/state", nil))
	if rr.Code != http.StatusForbidden {
		t.Errorf("missing cookie: want 403, got %d", rr.Code)
	}
	// Wrong cookie value → 403.
	rr = httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/state", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: "nope"})
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("wrong cookie: want 403, got %d", rr.Code)
	}
	// The boot token is not a session — it must not work against the API.
	rr = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/api/state", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: "boot-secret"})
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("boot token as session: want 403, got %d", rr.Code)
	}
	// Non-loopback Origin → 403 even with a valid session.
	rr = httptest.NewRecorder()
	req = withSession(httptest.NewRequest("GET", "/api/state", nil))
	req.Header.Set("Origin", "http://evil.example.com")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("evil origin: want 403, got %d", rr.Code)
	}
	// Valid session → 200.
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, withSession(httptest.NewRequest("GET", "/api/state", nil)))
	if rr.Code != http.StatusOK {
		t.Errorf("good session: want 200, got %d", rr.Code)
	}
}

func TestBootTokenExchange(t *testing.T) {
	p := newTestPicker()
	h := p.routes()

	// No session, no boot token → 403.
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/", nil))
	if rr.Code != http.StatusForbidden {
		t.Errorf("bare index: want 403, got %d", rr.Code)
	}

	// Valid boot token → session cookie + redirect to a clean URL.
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/?t=boot-secret", nil))
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("exchange: want 303, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/" {
		t.Errorf("redirect location = %q, want %q (token must not survive in the URL)", loc, "/")
	}
	var cookie *http.Cookie
	for _, c := range rr.Result().Cookies() {
		if c.Name == sessionCookie {
			cookie = c
		}
	}
	if cookie == nil || cookie.Value != "secret" {
		t.Fatalf("expected %s cookie with the api token, got %+v", sessionCookie, cookie)
	}
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Errorf("cookie should be HttpOnly + SameSite=Strict, got %+v", cookie)
	}

	// The boot token is single-use: replaying the launch URL → 403.
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/?t=boot-secret", nil))
	if rr.Code != http.StatusForbidden {
		t.Errorf("boot token replay: want 403, got %d", rr.Code)
	}

	// The issued session loads the page (reloads keep working).
	rr = httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(cookie)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "Pick what to back up") {
		t.Errorf("session index: want 200 with picker HTML, got %d", rr.Code)
	}
}

func TestIsLoopbackOrigin(t *testing.T) {
	cases := map[string]bool{
		"http://localhost:8080":         true,
		"http://127.0.0.1:39000":        true,
		"http://[::1]:8080":             true,
		"http://127.9.9.9":              true, // whole 127/8 is loopback
		"https://localhost":             true,
		"http://localhost.attacker.com": false, // substring traps
		"http://evil.com/?x=127.0.0.1":  false,
		"http://127.0.0.1.evil.com":     false,
		"http://192.168.1.10:8080":      false,
		"null":                          false, // sandboxed-iframe Origin
		"file:///etc/passwd":            false,
		"":                              false,
	}
	for origin, want := range cases {
		if got := isLoopbackOrigin(origin); got != want {
			t.Errorf("isLoopbackOrigin(%q) = %v, want %v", origin, got, want)
		}
	}
}

func TestMutateUpdatesState(t *testing.T) {
	p := newTestPicker()
	h := p.routes()

	mutate := func(op, value string) {
		body := strings.NewReader(`{"op":"` + op + `","value":"` + value + `"}`)
		h.ServeHTTP(httptest.NewRecorder(), withSession(httptest.NewRequest("POST", "/api/mutate", body)))
	}
	mutate("exclude", "/home/x/Downloads")
	mutate("include", "/data")

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, withSession(httptest.NewRequest("GET", "/api/state", nil)))

	var st struct {
		Includes []string `json:"includes"`
		Excludes []string `json:"excludes"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if !contains(st.Includes, "/data") || !contains(st.Includes, "/home/x") {
		t.Errorf("includes missing entries: %v", st.Includes)
	}
	if !contains(st.Excludes, "/home/x/Downloads") {
		t.Errorf("exclude not recorded: %v", st.Excludes)
	}
}

func TestIndexServesHTML(t *testing.T) {
	p := newTestPicker()
	rr := httptest.NewRecorder()
	p.routes().ServeHTTP(rr, withSession(httptest.NewRequest("GET", "/", nil)))
	if rr.Code != http.StatusOK {
		t.Fatalf("index: want 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Pick what to back up") {
		t.Error("index did not serve the embedded picker HTML")
	}
}

func TestListEndpoint(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "sub"), 0o755)
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte("hi"), 0o644)

	p := newTestPicker()
	p.home = dir
	rr := httptest.NewRecorder()
	p.routes().ServeHTTP(rr, withSession(httptest.NewRequest("GET", "/api/list?path="+dir, nil)))

	var res struct {
		Entries []struct {
			Name  string `json:"name"`
			IsDir bool   `json:"isDir"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Entries) != 2 || !res.Entries[0].IsDir || res.Entries[0].Name != "sub" {
		t.Errorf("unexpected listing: %+v", res.Entries)
	}
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

package diagnose

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCSRFDebug(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("REQ path=%s csrf-header=%q", r.URL.Path, r.Header.Get("X-CSRF-Token"))
		if strings.HasSuffix(r.URL.Path, "/api/login") {
			w.Header().Set("X-CSRF-Token", "csrf-1")
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[]}`))
			return
		}
		_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[]}`))
	}))
	defer srv.Close()

	client, err := newUnifiClient(UnifiCreds{Endpoint: srv.URL, Username: "a", Password: "b"})
	if err != nil {
		t.Fatal(err)
	}
	client.http.Transport = srv.Client().Transport
	client.legacy = true

	if err := client.login(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Logf("captured csrf = %q", client.csrf)
}

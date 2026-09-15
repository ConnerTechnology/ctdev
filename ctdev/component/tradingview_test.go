package component

import (
	"strings"
	"testing"
)

func TestTradingViewRegistryEntry(t *testing.T) {
	c := FindByName("tradingview")
	if c == nil {
		t.Fatal("tradingview not registered")
	}
	if c.Category != CategoryDesktop {
		t.Errorf("category = %q, want %q", c.Category, CategoryDesktop)
	}
	// macOS is found by the bundle, Linux by the /usr/bin symlink the .deb's
	// postinst creates — both halves must be declared or `ctdev info` reports
	// the app missing on one platform.
	if c.DetectCmd != "tradingview" {
		t.Errorf("DetectCmd = %q, want tradingview", c.DetectCmd)
	}
	if len(c.DetectApps) != 1 || c.DetectApps[0] != tradingviewApp {
		t.Errorf("DetectApps = %v, want [%s]", c.DetectApps, tradingviewApp)
	}
}

func TestTradingViewDebIsAmd64(t *testing.T) {
	// The installer refuses other architectures up front; the URL must agree
	// with that check or the refusal is wrong on one side.
	if !strings.HasSuffix(tradingviewDebURL, "_amd64.deb") {
		t.Errorf("deb URL %q is not the amd64 build", tradingviewDebURL)
	}
}

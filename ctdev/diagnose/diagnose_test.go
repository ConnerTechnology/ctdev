package diagnose

import (
	"context"
	"net/netip"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestSeverityRank(t *testing.T) {
	got := []Severity{OK, Skipped, Warn, Info, Fail}
	slices.SortFunc(got, func(a, b Severity) int { return a.Rank() - b.Rank() })

	want := []Severity{Fail, Warn, Info, OK, Skipped}
	if !slices.Equal(got, want) {
		t.Errorf("sorted by Rank = %v, want %v", got, want)
	}
}

func TestSelect(t *testing.T) {
	catalog := []Check{
		{ID: "plain"},
		{ID: "net", Network: true},
		{ID: "slow", Deep: true},
		{ID: "slow-net", Deep: true, Network: true},
	}

	ids := func(checks []Check) []string {
		var out []string
		for _, c := range checks {
			out = append(out, c.ID)
		}
		return out
	}

	tests := []struct {
		name          string
		deep, network bool
		want          []string
	}{
		// `ctdev status` reuses the catalog with network off, which is what
		// keeps its "no network calls" contract honest.
		{"status", false, false, []string{"plain"}},
		{"default", false, true, []string{"plain", "net"}},
		{"deep", true, true, []string{"plain", "net", "slow", "slow-net"}},
		{"deep offline", true, false, []string{"plain", "slow"}},
	}
	for _, tt := range tests {
		if got := ids(Select(catalog, tt.deep, tt.network)); !slices.Equal(got, tt.want) {
			t.Errorf("%s: Select(deep=%v, network=%v) = %v, want %v",
				tt.name, tt.deep, tt.network, got, tt.want)
		}
	}
}

// A check that panics must cost you that row, not the report. On a machine
// you're only visiting, one surprising line of tool output is exactly the kind
// of thing that panics a parser.
func TestRunAllRecoversPanic(t *testing.T) {
	checks := []Check{
		{ID: "boom", Run: func(context.Context, Facts) Result { panic("unexpected output") }},
		{ID: "fine", Run: func(context.Context, Facts) Result { return ok("all good") }},
	}

	results := RunAll(context.Background(), checks, Facts{})
	if len(results) != 2 {
		t.Fatalf("expected both checks to report, got %d", len(results))
	}
	if got := results["boom"].Severity; got != Skipped {
		t.Errorf("panicking check severity = %v, want Skipped", got)
	}
	if !strings.Contains(results["boom"].Detail, "unexpected output") {
		t.Errorf("panic detail = %q, want it to name the panic", results["boom"].Detail)
	}
	if got := results["fine"].Severity; got != OK {
		t.Errorf("neighbouring check severity = %v, want OK", got)
	}
}

// A hung probe must not stall the run either — a black-holed resolver or a
// wedged controller is a common enough failure that it has to be bounded.
func TestRunAllTimesOutSlowCheck(t *testing.T) {
	old := checkTimeout
	checkTimeout = 50 * time.Millisecond
	defer func() { checkTimeout = old }()

	checks := []Check{
		{ID: "hang", Run: func(ctx context.Context, _ Facts) Result {
			<-ctx.Done()
			// Deliberately return a healthy result after the deadline: the
			// point is that RunAll has already moved on.
			return ok("finished late")
		}},
		{ID: "fast", Run: func(context.Context, Facts) Result { return ok("quick") }},
	}

	start := time.Now()
	results := RunAll(context.Background(), checks, Facts{})
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("RunAll took %s, want it bounded by the check timeout", elapsed)
	}
	if got := results["hang"].Severity; got != Skipped {
		t.Errorf("hung check severity = %v, want Skipped", got)
	}
	if got := results["fast"].Severity; got != OK {
		t.Errorf("fast check severity = %v, want OK", got)
	}
}

func TestRunAllSkipsNilRun(t *testing.T) {
	results := RunAll(context.Background(), []Check{{ID: "no-op"}}, Facts{})
	if len(results) != 0 {
		t.Errorf("a check with no Run should produce no result, got %v", results)
	}
}

func TestGroupsInOrder(t *testing.T) {
	byGroup := map[string][]Check{
		GroupSystem:   {{}},
		GroupNetwork:  {{}},
		"Zebra":       {{}},
		GroupInternet: {{}},
		"Alpaca":      {{}},
		GroupHardware: {},
	}

	got := groupsInOrder(byGroup)
	want := []string{GroupNetwork, GroupInternet, GroupSystem, "Alpaca", "Zebra"}
	if !slices.Equal(got, want) {
		t.Errorf("groupsInOrder = %v, want %v (canonical order first, then extras alphabetically, empty groups dropped)", got, want)
	}
}

func TestCheckLocalAddress(t *testing.T) {
	tests := []struct {
		name string
		ip   string
		want Severity
	}{
		{"no address at all", "", Fail},
		// The single most diagnostic address a machine can have.
		{"APIPA means DHCP failed", "169.254.9.12", Fail},
		{"ordinary private lease", "192.168.1.42", OK},
		{"carrier-grade NAT", "100.64.0.7", OK},
		{"public address on the interface", "203.0.113.9", Info},
	}
	for _, tt := range tests {
		f := Facts{Iface: "wlan0", IsWiFi: true}
		if tt.ip != "" {
			f.LocalIP = netip.MustParseAddr(tt.ip)
		}
		if got := checkLocalAddress(context.Background(), f).Severity; got != tt.want {
			t.Errorf("%s: severity = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestCheckLocalAddressAdvisesOnFailure(t *testing.T) {
	// Every Fail has to leave the reader with something to do — that's the
	// difference between this and `ip addr`.
	res := checkLocalAddress(context.Background(), Facts{
		LocalIP: netip.MustParseAddr("169.254.9.12"),
		Iface:   "wlan0",
		IsWiFi:  true,
	})
	if res.Advice == "" {
		t.Error("an APIPA address should come with a recommendation")
	}
	if !strings.Contains(res.Detail, "DHCP") {
		t.Errorf("detail = %q, want it to name DHCP as the cause", res.Detail)
	}
}

func TestDescribeLink(t *testing.T) {
	tests := []struct {
		f    Facts
		want string
	}{
		{Facts{Iface: "wlp5s0", IsWiFi: true}, "wlp5s0 (Wi-Fi)"},
		{Facts{Iface: "enp7s0"}, "enp7s0 (wired)"},
		{Facts{}, "an unknown interface"},
	}
	for _, tt := range tests {
		if got := describeLink(tt.f); got != tt.want {
			t.Errorf("describeLink(%+v) = %q, want %q", tt.f, got, tt.want)
		}
	}
}

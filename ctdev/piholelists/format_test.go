package piholelists

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseReadsEveryKindAndFillsDefaults(t *testing.T) {
	src := `
adlists = [
  { url = "https://example.com/ads.txt", comment = "primary list" },
]

allow = [
  { domain = "s1.sentry-cdn.com" },
]

deny = [
  { domain = "tracker.example", enabled = false },
]

allow_regex = [
  { pattern = '^([a-z0-9-]+\.)?sentry\.io$', comment = "dashboard only" },
]

deny_regex = [
  { pattern = '(\.|^)reddit\.com$', groups = ["Kids", "Default"] },
]
`
	got, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := Lists{
		{Kind: Adlist, Key: "https://example.com/ads.txt", Comment: "primary list", Groups: []string{"Default"}, Enabled: true},
		{Kind: Allow, Key: "s1.sentry-cdn.com", Groups: []string{"Default"}, Enabled: true},
		{Kind: Deny, Key: "tracker.example", Groups: []string{"Default"}},
		{Kind: AllowRegex, Key: `^([a-z0-9-]+\.)?sentry\.io$`, Comment: "dashboard only", Groups: []string{"Default"}, Enabled: true},
		{Kind: DenyRegex, Key: `(\.|^)reddit\.com$`, Groups: []string{"Default", "Kids"}, Enabled: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func TestParseRejectsBadEntries(t *testing.T) {
	cases := map[string]string{
		"missing key":    `allow = [ { comment = "no domain" } ]`,
		"wrong key":      `allow = [ { url = "https://example.com" } ]`,
		"duplicate":      `allow = [ { domain = "a.example" }, { domain = "a.example" } ]`,
		"unknown field":  `allow = [ { domain = "a.example", note = "x" } ]`,
		"unknown table":  `allowed = [ { domain = "a.example" } ]`,
		"blank group":    `allow = [ { domain = "a.example", groups = ["  "] } ]`,
		"malformed toml": `allow = [`,
	}
	for name, src := range cases {
		if _, err := Parse([]byte(src)); err == nil {
			t.Errorf("%s: expected an error, got none", name)
		}
	}
}

func TestParseAllowsSameKeyInDifferentKinds(t *testing.T) {
	got, err := Parse([]byte("allow = [ { domain = \"a.example\" } ]\ndeny = [ { domain = \"a.example\" } ]"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 entries, got %d", len(got))
	}
}

func TestEncodeOmitsDefaultsAndSortsEntries(t *testing.T) {
	out := string(Encode(Lists{
		{Kind: Allow, Key: "z.example", Groups: []string{"Default"}, Enabled: true},
		{Kind: Allow, Key: "a.example", Comment: `he said "hi"`, Enabled: true},
		{Kind: DenyRegex, Key: `(\.|^)reddit\.com$`, Groups: []string{"Kids"}},
	}))

	for _, want := range []string{
		"\nallow = [\n  { domain = \"a.example\", comment = \"he said \\\"hi\\\"\", groups = [] },\n  { domain = \"z.example\" },\n]\n",
		"\ndeny = [\n]\n",
		"\ndeny_regex = [\n  { pattern = '(\\.|^)reddit\\.com$', groups = [\"Kids\"], enabled = false },\n]\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Encode output missing\n%q\ngot:\n%s", want, out)
		}
	}
	if !strings.HasPrefix(out, "# Pi-hole lists") {
		t.Errorf("Encode should start with the explanatory header, got:\n%s", out)
	}
}

func TestEncodeQuotesPatternsContainingApostrophes(t *testing.T) {
	out := string(Encode(Lists{{Kind: AllowRegex, Key: `it's\.bad$`, Enabled: true, Groups: []string{"Default"}}}))
	if !strings.Contains(out, `pattern = "it's\\.bad$"`) {
		t.Errorf("a pattern with an apostrophe needs a basic string, got:\n%s", out)
	}
}

func TestRoundTrip(t *testing.T) {
	want := Lists{
		{Kind: Adlist, Key: "https://example.com/ads.txt", Comment: "primary", Groups: []string{"Default"}, Enabled: true},
		{Kind: Allow, Key: "a.example", Groups: []string{"Default"}, Enabled: true},
		{Kind: Deny, Key: "b.example", Comment: "off for now", Groups: []string{"Kids"}},
		{Kind: AllowRegex, Key: `^a\.example$`, Groups: nil, Enabled: true},
		{Kind: DenyRegex, Key: `it's\.bad$`, Comment: `quote " and backslash \`, Groups: []string{"Default", "Kids"}, Enabled: true},
	}
	got, err := Parse(Encode(want))
	if err != nil {
		t.Fatalf("Parse(Encode(...)): %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round trip mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

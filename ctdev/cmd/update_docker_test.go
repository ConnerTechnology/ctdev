package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ConnerTechnology/dotfiles/ctdev/component"
)

// The compose stacks ctdev manages (pihole, caddy, beszel, portainer) are all
// declared Linux-only, but IsInstalled() is a bare os.Stat on the compose file.
// Without an OS gate a Mac holding a restored ~/pihole/ would be scanned — and
// offered updates — for a stack it isn't running.
func TestDockerComposeStacksAreLinuxOnly(t *testing.T) {
	if got := dockerComposeStacksFor(component.OSMacOS); len(got) != 0 {
		t.Errorf("expected no compose stacks on macOS, got %v", got)
	}

	// Sanity-check the fixture assumption: the registry really does declare
	// compose stacks, and they really are Linux-only. Otherwise the assertion
	// above would pass for the wrong reason.
	var composeComponents int
	for i := range component.Registry {
		c := &component.Registry[i]
		if filepath.Base(c.DetectPath) != "docker-compose.yml" {
			continue
		}
		composeComponents++
		if c.SupportsOS(component.OSMacOS) {
			t.Errorf("%s is a compose stack that claims macOS support; the gate needs revisiting", c.Name)
		}
	}
	if composeComponents == 0 {
		t.Fatal("no compose-stack components in the registry — this test proves nothing")
	}
}

func TestParseBuildxDigest(t *testing.T) {
	out := `Name:      docker.io/pihole/pihole:latest
MediaType: application/vnd.oci.image.index.v1+json
Digest:    sha256:8ea95136e7c8c15b42d88eadf1a3875421aa1be30ad39e50f661188cc986fb27

Manifests:
`
	got := parseBuildxDigest(out)
	want := "sha256:8ea95136e7c8c15b42d88eadf1a3875421aa1be30ad39e50f661188cc986fb27"
	if got != want {
		t.Errorf("parseBuildxDigest = %q, want %q", got, want)
	}
	if d := parseBuildxDigest("Name: x\nMediaType: y\n"); d != "" {
		t.Errorf("missing digest should yield \"\", got %q", d)
	}
}

func TestImageRepo(t *testing.T) {
	cases := map[string]string{
		"pihole/pihole:latest":                "pihole/pihole",
		"caddy-homelab:local":                 "caddy-homelab",
		"klutchell/unbound:latest":            "klutchell/unbound",
		"ghcr.io/owner/app:v1.2.3":            "ghcr.io/owner/app",
		"registry:5000/team/app:tag":          "registry:5000/team/app",
		"portainer/portainer-ce":              "portainer/portainer-ce",
		"pihole/pihole@sha256:abc":            "pihole/pihole",
		"registry:5000/team/app@sha256:deadb": "registry:5000/team/app",
	}
	for in, want := range cases {
		if got := imageRepo(in); got != want {
			t.Errorf("imageRepo(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPickDigest(t *testing.T) {
	rds := []string{
		"pihole/pihole@sha256:aaa",
		"otherrepo/pihole@sha256:bbb",
	}
	if d, ok := pickDigest(rds, "pihole/pihole"); !ok || d != "sha256:aaa" {
		t.Errorf("matched repo: got (%q,%v), want (sha256:aaa,true)", d, ok)
	}
	// Single entry falls back even when the repo doesn't match.
	if d, ok := pickDigest([]string{"x/y@sha256:ccc"}, "no/match"); !ok || d != "sha256:ccc" {
		t.Errorf("single fallback: got (%q,%v), want (sha256:ccc,true)", d, ok)
	}
	// Empty list (locally-built image) yields not-ok.
	if _, ok := pickDigest(nil, "any"); ok {
		t.Error("empty RepoDigests should yield ok=false")
	}
	// Multiple, none matching -> not ok.
	if _, ok := pickDigest(rds, "no/match"); ok {
		t.Error("no match among many should yield ok=false")
	}
}

func TestParseDockerfileFroms(t *testing.T) {
	dockerfile := `# comment
FROM caddy:2-builder AS builder
RUN xcaddy build --with github.com/caddy-dns/cloudflare

FROM --platform=$BUILDPLATFORM caddy:2
COPY --from=builder /usr/bin/caddy /usr/bin/caddy
`
	got := parseDockerfileFroms(dockerfile)
	want := []string{"caddy:2-builder", "caddy:2"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseDockerfileFroms = %v, want %v", got, want)
	}

	// References to a named stage and scratch are excluded; duplicates collapse.
	multi := `FROM golang:1.22 AS build
FROM build AS test
FROM scratch
FROM golang:1.22
`
	got = parseDockerfileFroms(multi)
	want = []string{"golang:1.22"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseDockerfileFroms(multi) = %v, want %v", got, want)
	}
}

func TestDigestMarkerRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), baseDigestMarker)
	in := map[string]string{
		"caddy:2":         "sha256:aaa",
		"caddy:2-builder": "sha256:bbb",
	}
	writeDigestMarker(path, in)
	got := readDigestMarker(path)
	if !reflect.DeepEqual(got, in) {
		t.Errorf("round trip = %v, want %v", got, in)
	}
	// Missing file -> empty map, never nil.
	if m := readDigestMarker(filepath.Join(t.TempDir(), "nope")); m == nil || len(m) != 0 {
		t.Errorf("missing marker should be empty map, got %v", m)
	}
}

// A base image's index digest moves whenever Docker Hub re-pushes the index —
// which it does when any architecture is rebuilt, and even when only the index
// annotations change. Tracking it made `ctdev update` offer a caddy rebuild
// that rebuilt nothing. What matters is the manifest for this host's platform.
func TestPickPlatformDigest(t *testing.T) {
	// Trimmed from `docker buildx imagetools inspect caddy:2-builder --raw`:
	// two real platforms plus the attestation manifest, which carries
	// platform unknown/unknown and must never be picked.
	raw := []byte(`{
	  "mediaType": "application/vnd.oci.image.index.v1+json",
	  "manifests": [
	    {"digest": "sha256:aaa", "platform": {"architecture": "amd64", "os": "linux"}},
	    {"digest": "sha256:bbb", "platform": {"architecture": "arm64", "os": "linux", "variant": "v8"}},
	    {"digest": "sha256:ccc", "platform": {"architecture": "unknown", "os": "unknown"}}
	  ]
	}`)

	if got := pickPlatformDigest(raw, "linux", "arm64"); got != "sha256:bbb" {
		t.Errorf("arm64 digest = %q, want sha256:bbb", got)
	}
	if got := pickPlatformDigest(raw, "linux", "amd64"); got != "sha256:aaa" {
		t.Errorf("amd64 digest = %q, want sha256:aaa", got)
	}
	// No entry for this platform, and a single-arch image (a bare manifest with
	// no "manifests" array) both fall back to the index digest.
	if got := pickPlatformDigest(raw, "linux", "riscv64"); got != "" {
		t.Errorf("absent platform = %q, want \"\"", got)
	}
	single := []byte(`{"mediaType":"application/vnd.oci.image.manifest.v1+json","layers":[]}`)
	if got := pickPlatformDigest(single, "linux", "arm64"); got != "" {
		t.Errorf("single manifest = %q, want \"\"", got)
	}
	if got := pickPlatformDigest([]byte("not json"), "linux", "arm64"); got != "" {
		t.Errorf("garbage = %q, want \"\"", got)
	}
}

// Markers written before the switch hold index digests, which never match a
// platform digest. Reading one as empty re-seeds the baseline instead of
// reporting a rebuild that isn't needed.
func TestDigestMarkerIgnoresOlderFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".ctdev-base-digests")
	old := "# ctdev: remote base-image digests at last build\ncaddy:2 sha256:old\n"
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readDigestMarker(path); len(got) != 0 {
		t.Errorf("older-format marker should read as empty, got %v", got)
	}

	// A marker this version wrote still round-trips.
	writeDigestMarker(path, map[string]string{"caddy:2": "sha256:new"})
	if got := readDigestMarker(path)["caddy:2"]; got != "sha256:new" {
		t.Errorf("round-trip = %q, want sha256:new", got)
	}
}

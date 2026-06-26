package cmd

import (
	"path/filepath"
	"reflect"
	"testing"
)

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

package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ConnerTechnology/dotfiles/ctdev/component"
	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/checklist"
)

// baseDigestMarker is written into a built stack's directory recording the
// remote digests of its Dockerfile base images at the last build. `ctdev
// update` compares the current remote digests against it to tell whether a
// locally-built image (e.g. caddy) is worth rebuilding — the built image itself
// carries no registry digest to compare.
const baseDigestMarker = ".ctdev-base-digests"

// dockerStack is an installed docker-compose stack discovered from the
// component registry (any component whose DetectPath is a docker-compose.yml).
type dockerStack struct {
	Name    string // component name, e.g. "pihole"
	Dir     string // directory holding docker-compose.yml (+ Dockerfile for built stacks)
	Compose string // full path to docker-compose.yml
}

// dockerComposeStacks returns the installed compose stacks ctdev manages on
// this node. A stack is any installed component whose DetectPath points at a
// docker-compose.yml (pihole, caddy, beszel, portainer, ...).
func dockerComposeStacks() []dockerStack {
	return dockerComposeStacksFor(component.OS(platform.Detect().OS))
}

// dockerComposeStacksFor takes the OS as a parameter so it can be tested;
// platform.Detect() is sync.Once-cached and can't be injected.
func dockerComposeStacksFor(osType component.OS) []dockerStack {
	var stacks []dockerStack
	for i := range component.Registry {
		c := &component.Registry[i]
		if !strings.HasSuffix(c.DetectPath, "docker-compose.yml") {
			continue
		}
		// IsInstalled is a bare os.Stat on DetectPath, so without this gate a Mac
		// holding a restored ~/pihole/docker-compose.yml would be scanned — and
		// offered updates — for a Linux-only stack it isn't running.
		if !c.SupportsOS(osType) {
			continue
		}
		if !c.IsInstalled() {
			continue
		}
		compose := os.ExpandEnv(c.DetectPath)
		stacks = append(stacks, dockerStack{
			Name:    c.Name,
			Dir:     filepath.Dir(compose),
			Compose: compose,
		})
	}
	return stacks
}

// scanDocker reports compose stacks with a newer image available. Registry
// images are compared by digest (local RepoDigest vs the registry's current
// index digest); locally-built images are tracked through their Dockerfile base
// images. It never pulls. Returns an error only when no stack could be checked,
// so a single flaky image doesn't discard the updates we did find.
func scanDocker(ctx context.Context) ([]checklist.UpdateItem, error) {
	if _, err := exec.LookPath("docker"); err != nil {
		return nil, nil
	}
	stacks := dockerComposeStacks()
	if len(stacks) == 0 {
		return nil, nil
	}

	var items []checklist.UpdateItem
	var errs []string
	for _, s := range stacks {
		images, err := composeImages(ctx, s.Compose)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", s.Name, err))
			continue
		}
		var stale []string
		for _, img := range images {
			changed, label, err := imageHasUpdate(ctx, s, img)
			if err != nil {
				errs = append(errs, fmt.Sprintf("%s (%s): %v", s.Name, img, err))
				continue
			}
			if changed {
				stale = append(stale, label)
			}
		}
		if len(stale) > 0 {
			items = append(items, checklist.UpdateItem{
				Name:   s.Name,
				Source: "docker",
				NewVer: strings.Join(stale, ", "),
			})
		}
	}

	// Surface failures only when nothing was found — otherwise returning an error
	// would make scanAll drop the updates we did detect.
	if len(items) == 0 && len(errs) > 0 {
		return nil, fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return items, nil
}

// dockerProbeTimeout bounds each docker call made while scanning. The scan runs
// behind a single-line spinner, so an unbounded call is indistinguishable from a
// hang: `docker image inspect` blocks when Docker Desktop isn't running, and
// `buildx imagetools inspect` is a network round-trip per image.
const dockerProbeTimeout = 20 * time.Second

// composeImages lists the images a stack resolves to (one per service).
func composeImages(ctx context.Context, compose string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, dockerProbeTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", "compose", "-f", compose, "config", "--images").Output()
	if err != nil {
		return nil, fmt.Errorf("compose config: %w", err)
	}
	var images []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			images = append(images, line)
		}
	}
	return images, nil
}

// imageHasUpdate reports whether img has a newer build available and a label for
// the update list. An image with no local registry digest is treated as
// locally-built and checked via its Dockerfile base images instead.
func imageHasUpdate(ctx context.Context, s dockerStack, img string) (bool, string, error) {
	// A locally-built image (caddy-homelab:local) is not in any registry, yet
	// modern BuildKit still stamps it with a RepoDigest — so the RepoDigest
	// heuristic below can't distinguish it from a pulled image and would try to
	// `imagetools inspect` it, failing with an auth/not-found error. The
	// authoritative signal is whether the stack has a Dockerfile: build it from
	// its base images instead.
	if stackIsBuilt(s) {
		return builtImageHasUpdate(ctx, s, img)
	}
	local, ok := localIndexDigest(ctx, img)
	if !ok {
		return builtImageHasUpdate(ctx, s, img)
	}
	remote, err := remoteIndexDigest(ctx, img)
	if err != nil {
		return false, "", err
	}
	if remote == "" || remote == local {
		return false, "", nil
	}
	return true, img, nil
}

// builtImageHasUpdate detects updates for a locally-built image by watching the
// remote digests of its Dockerfile base images against a per-stack marker. The
// first time a base is seen it is recorded as the baseline (not flagged); a
// later digest change flags a rebuild. The marker is only refreshed for changed
// bases after an actual rebuild (see refreshBaseDigestMarker), so a pending
// update keeps showing until acted on.
func builtImageHasUpdate(ctx context.Context, s dockerStack, img string) (bool, string, error) {
	bases := dockerfileBaseImages(s.Dir)
	if len(bases) == 0 {
		return false, "", nil
	}
	marker := filepath.Join(s.Dir, baseDigestMarker)
	recorded := readDigestMarker(marker)

	changed := false
	seeded := false
	for _, b := range bases {
		remote, err := remoteIndexDigest(ctx, b)
		if err != nil || remote == "" {
			continue // can't determine this base right now; skip it
		}
		if old, ok := recorded[b]; !ok {
			recorded[b] = remote // first sighting → baseline, don't flag
			seeded = true
		} else if old != remote {
			changed = true
		}
	}
	// Persist newly-seeded baselines so we don't re-evaluate them as "new" next
	// run. Stay read-only under --check / --dry-run.
	if seeded && !flagCheck && !flagDryRun {
		writeDigestMarker(marker, recorded)
	}
	if changed {
		return true, img + " (rebuild)", nil
	}
	return false, "", nil
}

// localIndexDigest returns the registry index digest the local image was pulled
// from (its RepoDigest). ok is false for locally-built or absent images, which
// have no RepoDigest to compare.
func localIndexDigest(ctx context.Context, img string) (digest string, ok bool) {
	ctx, cancel := context.WithTimeout(ctx, dockerProbeTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", "image", "inspect", img, "--format", "{{json .RepoDigests}}").Output()
	if err != nil {
		return "", false
	}
	var repoDigests []string
	if err := json.Unmarshal(bytes.TrimSpace(out), &repoDigests); err != nil {
		return "", false
	}
	return pickDigest(repoDigests, imageRepo(img))
}

// remoteIndexDigest returns the current index digest for img in its registry,
// via buildx imagetools (no pull). Returns "" if the digest line is absent.
func remoteIndexDigest(ctx context.Context, img string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, dockerProbeTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", "buildx", "imagetools", "inspect", img).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("imagetools inspect: %s", strings.TrimSpace(string(out)))
	}
	return parseBuildxDigest(string(out)), nil
}

// updateDockerStack updates one selected compose stack. Registry-image stacks
// are pulled and recreated; built stacks (those with a Dockerfile) are rebuilt
// from a freshly-pulled base, then their base-digest marker is refreshed.
func updateDockerStack(ctx context.Context, o sysutil.Opts, item checklist.UpdateItem) error {
	s, ok := stackByName(item.Name)
	if !ok {
		return fmt.Errorf("unknown stack %q", item.Name)
	}

	if stackIsBuilt(s) {
		// caddy and friends: rebuild against a newly-pulled base, then restart.
		// Mirrors the sudo path the caddy installer uses for this stack.
		if err := sysutil.SudoRun(ctx, o, "docker", "compose", "-f", s.Compose, "build", "--pull"); err != nil {
			return fmt.Errorf("build: %w", err)
		}
		if err := sysutil.SudoRun(ctx, o, "docker", "compose", "-f", s.Compose, "up", "-d"); err != nil {
			return fmt.Errorf("up: %w", err)
		}
		if !o.DryRun {
			refreshBaseDigestMarker(ctx, s)
		}
		return nil
	}

	if err := sysutil.Run(ctx, o, "docker", "compose", "-f", s.Compose, "pull"); err != nil {
		return fmt.Errorf("pull: %w", err)
	}
	return sysutil.Run(ctx, o, "docker", "compose", "-f", s.Compose, "up", "-d")
}

// stackByName resolves a stack name (an UpdateItem.Name) back to its stack.
func stackByName(name string) (dockerStack, bool) {
	for _, s := range dockerComposeStacks() {
		if s.Name == name {
			return s, true
		}
	}
	return dockerStack{}, false
}

// stackIsBuilt reports whether the stack builds its image locally (has a
// Dockerfile alongside the compose file) rather than pulling it.
func stackIsBuilt(s dockerStack) bool {
	_, err := os.Stat(filepath.Join(s.Dir, "Dockerfile"))
	return err == nil
}

// refreshBaseDigestMarker records the current remote base-image digests after a
// rebuild, so the stack stops being flagged until a base moves again.
func refreshBaseDigestMarker(ctx context.Context, s dockerStack) {
	bases := dockerfileBaseImages(s.Dir)
	if len(bases) == 0 {
		return
	}
	marker := filepath.Join(s.Dir, baseDigestMarker)
	m := readDigestMarker(marker)
	for _, b := range bases {
		if remote, err := remoteIndexDigest(ctx, b); err == nil && remote != "" {
			m[b] = remote
		}
	}
	writeDigestMarker(marker, m)
}

// dockerfileBaseImages returns the external base images a Dockerfile builds
// FROM (skipping references to earlier named stages and scratch).
func dockerfileBaseImages(dir string) []string {
	b, err := os.ReadFile(filepath.Join(dir, "Dockerfile"))
	if err != nil {
		return nil
	}
	return parseDockerfileFroms(string(b))
}

// --- pure helpers (unit-tested) ---

// parseBuildxDigest extracts the "Digest: sha256:..." line from
// `docker buildx imagetools inspect` output.
func parseBuildxDigest(out string) string {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Digest:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "Digest:"))
		}
	}
	return ""
}

// imageRepo strips the tag and any digest from an image reference, leaving the
// repository. A ':' only counts as a tag separator in the final path segment so
// a registry host:port is preserved (registry:5000/img:tag -> registry:5000/img).
func imageRepo(ref string) string {
	if i := strings.Index(ref, "@"); i >= 0 {
		ref = ref[:i]
	}
	slash := strings.LastIndex(ref, "/")
	if colon := strings.LastIndex(ref, ":"); colon > slash {
		ref = ref[:colon]
	}
	return ref
}

// pickDigest returns the digest from a RepoDigests list matching repo, falling
// back to the sole entry when there is exactly one.
func pickDigest(repoDigests []string, repo string) (string, bool) {
	for _, rd := range repoDigests {
		if name, digest, ok := strings.Cut(rd, "@"); ok && name == repo {
			return digest, true
		}
	}
	if len(repoDigests) == 1 {
		if _, digest, ok := strings.Cut(repoDigests[0], "@"); ok {
			return digest, true
		}
	}
	return "", false
}

// parseDockerfileFroms returns the external base images referenced by FROM
// directives, in order, de-duplicated. References to a previously-declared
// stage alias (FROM x AS name) and "scratch" are excluded.
func parseDockerfileFroms(content string) []string {
	var bases []string
	seen := map[string]bool{}
	stages := map[string]bool{}
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || !strings.EqualFold(fields[0], "FROM") {
			continue
		}
		i := 1
		for i < len(fields) && strings.HasPrefix(fields[i], "--") {
			i++ // skip flags like --platform=linux/arm64
		}
		if i >= len(fields) {
			continue
		}
		base := fields[i]
		if i+2 < len(fields) && strings.EqualFold(fields[i+1], "AS") {
			stages[fields[i+2]] = true
		}
		if stages[base] || base == "scratch" {
			continue
		}
		if !seen[base] {
			seen[base] = true
			bases = append(bases, base)
		}
	}
	return bases
}

// readDigestMarker parses an "image digest" per line marker file. Missing or
// unreadable files yield an empty map.
func readDigestMarker(path string) map[string]string {
	m := map[string]string{}
	b, err := os.ReadFile(path)
	if err != nil {
		return m
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if k, v, ok := strings.Cut(line, " "); ok {
			m[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return m
}

// writeDigestMarker writes the marker file, sorted for a stable diff.
func writeDigestMarker(path string, m map[string]string) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("# ctdev: remote base-image digests at last build; drives rebuild detection in 'ctdev update'\n")
	for _, k := range keys {
		fmt.Fprintf(&b, "%s %s\n", k, m[k])
	}
	_ = os.WriteFile(path, []byte(b.String()), 0o644)
}

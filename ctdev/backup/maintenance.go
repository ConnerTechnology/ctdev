package backup

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Every destination gets maintenance once a week, run by the hourly runner
// of whichever computer first finds it due (CTD-87, "Decided: the data
// check"). It forgets by retention, prunes, reads a rotating twentieth of
// the stored data, and test-downloads a file from each folder.
const (
	maintenanceInterval = 7 * 24 * time.Hour
	// Slices is how many parts the data check's rotation splits the stored
	// data into: one is read each week, so every pack is read every 20 weeks.
	Slices = 20
	// claimTTL is how long a computer's claim on a run stops others from
	// starting one. A claim older than this, with no result, was abandoned,
	// as by a computer that slept or crashed mid-run.
	claimTTL = 24 * time.Hour
)

// The steps of maintenance, as a failed DataCheck names them.
const (
	StepForget   = "forget"
	StepPrune    = "prune"
	StepCheck    = "check"
	StepDownload = "test download"
)

// The maintenance marker is a restic snapshot of one small file, made with
// backup --stdin, tagged tagMarker. Its record is also in a tag, base64url
// JSON after tagMarkerData, so reading every marker is one restic snapshots
// call with no dump. Being a snapshot, it's encrypted like the backups, and
// every key on the destination can read and write it, which a file at a
// reserved path in the bucket would need backend-specific code for.
//
// It carries no ctdev-path or ctdev-folder tag, so no folder's backup list
// matches it: Snapshots selects by folder ID, or for one-offs by path tag.
const (
	tagMarker     = "ctdev-maintenance"
	tagMarkerData = "ctdev-maintenance-data:"
	markerFile    = "ctdev-maintenance.json"
)

// DataCheck is one run of a destination's weekly maintenance: a claim while
// it runs, and its result once Finished is set.
type DataCheck struct {
	Host     string    `json:"host"`
	Started  time.Time `json:"started"`
	Finished time.Time `json:"finished,omitzero"`
	// Slice is k in check --read-data-subset=k/20: which twentieth of the
	// stored data was read.
	Slice int    `json:"slice"`
	OK    bool   `json:"ok"`
	Step  string `json:"step,omitempty"`  // the step that failed
	Error string `json:"error,omitempty"` // why
}

// Failed reports whether the run finished and failed.
func (c DataCheck) Failed() bool { return !c.Finished.IsZero() && !c.OK }

// Reason is the failure as one line: the step, then why.
func (c DataCheck) Reason() string {
	return "failed at " + c.Step + ": " + c.Error
}

// marker is one maintenance marker in the destination.
type marker struct {
	ID string
	DataCheck
}

// markers lists the destination's maintenance markers, oldest first.
func (d *Destination) markers(ctx context.Context) ([]marker, error) {
	out, err := d.restic(ctx, d.asComputer("", "snapshots", "--json", "--retry-lock", retryLock, "--tag", tagMarker))
	if err != nil {
		return nil, err
	}
	var raw []struct {
		ID   string   `json:"id"`
		Tags []string `json:"tags"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("read restic's snapshot list: %w", err)
	}
	var ms []marker
	for _, r := range raw {
		for _, t := range r.Tags {
			data, ok := strings.CutPrefix(t, tagMarkerData)
			if !ok {
				continue
			}
			var c DataCheck
			b, err := base64.RawURLEncoding.DecodeString(data)
			if err == nil {
				err = json.Unmarshal(b, &c)
			}
			if err != nil {
				return nil, fmt.Errorf("read the maintenance marker %s: %w", short(r.ID), err)
			}
			ms = append(ms, marker{ID: r.ID, DataCheck: c})
		}
	}
	slices.SortFunc(ms, func(a, b marker) int {
		return cmp.Or(a.Started.Compare(b.Started), cmp.Compare(a.ID, b.ID))
	})
	return ms, nil
}

// lastResult is the newest finished run among ms, or nil.
func lastResult(ms []marker) *marker {
	var last *marker
	for i := range ms {
		if !ms[i].Finished.IsZero() && (last == nil || ms[i].Started.After(last.Started)) {
			last = &ms[i]
		}
	}
	return last
}

// activeClaims are the runs among ms that started after the last result
// and haven't finished or been abandoned, earliest first.
func activeClaims(ms []marker, now time.Time) []marker {
	last := lastResult(ms)
	var claims []marker
	for _, mk := range ms {
		if !mk.Finished.IsZero() || now.Sub(mk.Started) >= claimTTL {
			continue
		}
		if last != nil && !mk.Started.After(last.Started) {
			continue
		}
		claims = append(claims, mk)
	}
	return claims
}

// maintenanceDue reports whether maintenance should start at now: the last
// run started 7 or more days ago, or there's none, and no computer is
// running one.
func maintenanceDue(ms []marker, now time.Time) bool {
	if last := lastResult(ms); last != nil && now.Sub(last.Started) < maintenanceInterval {
		return false
	}
	return len(activeClaims(ms, now)) == 0
}

// nextSlice is the slice after the last result's, wrapping after Slices.
func nextSlice(ms []marker) int {
	if last := lastResult(ms); last != nil && last.Slice >= 1 && last.Slice <= Slices {
		return last.Slice%Slices + 1
	}
	return 1
}

// maxMarkerError keeps a failure's reason, which goes in a tag, short.
const maxMarkerError = 400

// writeMarker saves c as a new marker and returns its ID.
func (d *Destination) writeMarker(ctx context.Context, c DataCheck) (string, error) {
	if len(c.Error) > maxMarkerError {
		c.Error = c.Error[:maxMarkerError] + "…"
	}
	data, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	r := d.asComputer("", "backup", "--stdin", "--stdin-filename", markerFile, "--json",
		"--retry-lock", retryLock, "--host", d.m.Hostname,
		"--tag", tagMarker, "--tag", tagMarkerData+base64.RawURLEncoding.EncodeToString(data))
	r.stdin = append(data, '\n')
	out, err := d.restic(ctx, r)
	if err != nil {
		return "", fmt.Errorf("write the maintenance marker: %w", err)
	}
	snap, err := backupSummary(out)
	if err != nil {
		return "", fmt.Errorf("write the maintenance marker: %w", err)
	}
	return snap.ID, nil
}

// LastDataCheck is the destination's newest finished maintenance, or nil
// when none has run yet. Every computer reads the same one.
func (d *Destination) LastDataCheck(ctx context.Context) (*DataCheck, error) {
	ms, err := d.markers(ctx)
	if err != nil {
		return nil, err
	}
	if last := lastResult(ms); last != nil {
		c := last.DataCheck
		return &c, nil
	}
	return nil, nil
}

// Maintenance is what one scheduled run found, and did, about a
// destination's weekly maintenance.
type Maintenance struct {
	Destination string
	Ran         bool       // this computer ran it
	Check       *DataCheck // the newest result, nil when none has run
	Err         error      // the run failed, or the marker couldn't be read or written
}

// maintainIfDue runs d's weekly maintenance when it's due and this computer
// wins the claim. folders are this computer's scheduled folders on d, whose
// keep rules the forget step applies.
func (m *Machine) maintainIfDue(ctx context.Context, d *Destination, folders []listedFolder) Maintenance {
	res := Maintenance{Destination: d.Name}
	ms, err := d.markers(ctx)
	if err != nil {
		res.Err = err
		return res
	}
	res.Check = resultOf(ms)
	if !maintenanceDue(ms, m.now()) {
		return res
	}
	claim := DataCheck{Host: m.Hostname, Started: m.now(), Slice: nextSlice(ms)}
	claimID, err := d.writeMarker(ctx, claim)
	if err != nil {
		res.Err = err
		return res
	}
	// Another computer may have found it due at the same time. Every
	// computer sees the same markers, and the earliest claim runs.
	if ms, err = d.markers(ctx); err != nil {
		res.Err = err
		return res
	}
	claims := activeClaims(ms, m.now())
	if last := lastResult(ms); (last != nil && m.now().Sub(last.Started) < maintenanceInterval) ||
		len(claims) == 0 || claims[0].ID != claimID {
		res.Check = resultOf(ms)
		if err := d.forget(ctx, claimID); err != nil {
			res.Err = fmt.Errorf("remove this computer's claim on the maintenance: %w", err)
		}
		return res
	}

	result := m.maintain(ctx, d, folders, claim)
	res.Ran, res.Check = true, &result
	if _, err := d.writeMarker(ctx, result); err != nil {
		res.Err = err
		return res
	}
	// The result replaces the markers before it, this run's claim included.
	// A later claim is another computer's, which removes its own.
	var old []string
	for _, mk := range ms {
		if !mk.Started.After(claim.Started) {
			old = append(old, mk.ID)
		}
	}
	if len(old) > 0 {
		if err := d.forget(ctx, old...); err != nil {
			res.Err = fmt.Errorf("remove the old maintenance markers: %w", err)
		}
	}
	if result.Failed() && res.Err == nil {
		res.Err = errors.New("data check " + result.Reason())
	}
	return res
}

func resultOf(ms []marker) *DataCheck {
	if last := lastResult(ms); last != nil {
		c := last.DataCheck
		return &c
	}
	return nil
}

// listedFolder is one of this computer's scheduled folders, with its config.
type listedFolder struct {
	Root string
	Config
}

// maintain runs the four steps in order, stopping at the first that fails.
func (m *Machine) maintain(ctx context.Context, d *Destination, folders []listedFolder, claim DataCheck) DataCheck {
	c := claim
	fail := func(step string, err error) DataCheck {
		c.Finished, c.Step, c.Error = m.now(), step, strings.ReplaceAll(err.Error(), "\n", "; ")
		return c
	}
	// 1. The keep rules of this computer's folders, again: a safety net for
	// a run whose retention failed, which this clears when it succeeds.
	var errs []error
	for _, f := range folders {
		if err := m.Retain(ctx, d, f.Root, f.Config); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", DisplayPath(m.Home, f.Root), err))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return fail(StepForget, err)
	}
	// 2. Prune frees the space of every forgotten backup. It takes the
	// exclusive lock, so backups wait for it (--retry-lock).
	if _, err := d.restic(ctx, d.asComputer("", "prune", "--retry-lock", retryLock)); err != nil {
		return fail(StepPrune, err)
	}
	// 3. Read this week's twentieth of the stored data.
	subset := "--read-data-subset=" + strconv.Itoa(c.Slice) + "/" + strconv.Itoa(Slices)
	check := d.asComputer("", "check", "--retry-lock", retryLock, subset)
	var stderr bytes.Buffer
	check.stderr = &stderr
	if _, err := d.restic(ctx, check); err != nil {
		return fail(StepCheck, checkFailure(stderr.Bytes(), err))
	}
	// 4. A small test restore: one random file from each folder's latest
	// backup reads back whole.
	if err := m.testDownload(ctx, d); err != nil {
		return fail(StepDownload, err)
	}
	c.Finished, c.OK = m.now(), true
	return c
}

// checkFailure says which packs restic check found damaged, from its
// "pack <id> contains N errors" lines; restic's last line only says the
// repository contains errors. Any other failure is err as it is.
func checkFailure(stderr []byte, err error) error {
	var packs []string
	sc := bufio.NewScanner(bytes.NewReader(stderr))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		rest, ok := strings.CutPrefix(sc.Text(), "pack ")
		if id, _, found := strings.Cut(rest, " contains "); ok && found {
			packs = append(packs, short(id))
		}
	}
	switch len(packs) {
	case 0:
		return err
	case 1:
		return fmt.Errorf("damaged pack %s: see restic's troubleshooting guide to repair it", packs[0])
	}
	return fmt.Errorf("%d damaged packs, %s: see restic's troubleshooting guide to repair them", len(packs), strings.Join(packs, ", "))
}

// testDownload dumps one random file from each folder's latest backup, from
// every computer, and checks that it reads back without error and is the
// size the backup recorded.
func (m *Machine) testDownload(ctx context.Context, d *Destination) error {
	latest, err := d.latestPerFolder(ctx)
	if err != nil {
		return err
	}
	for _, s := range latest {
		files, err := d.fileSizes(ctx, s.ID)
		if err != nil {
			return err
		}
		if len(files) == 0 {
			continue // an empty folder has nothing to read back
		}
		f := files[m.pick(len(files))]
		var n countWriter
		r := d.asComputer("", "dump", "--retry-lock", retryLock, s.ID, f.path)
		r.stdout = &n
		if _, err := d.restic(ctx, r); err != nil {
			return fmt.Errorf("%s from the backup of %s: %w", f.path, s.Path, err)
		}
		if int64(n) != f.size {
			return fmt.Errorf("%s from the backup of %s read back as %d bytes, but the backup recorded %d", f.path, s.Path, n, f.size)
		}
	}
	return nil
}

// pick chooses one of n things at random: a file to test-download.
func (m *Machine) pick(n int) int {
	if m.pickFn != nil {
		return m.pickFn(n)
	}
	return rand.IntN(n)
}

type countWriter int64

func (w *countWriter) Write(p []byte) (int, error) {
	*w += countWriter(len(p))
	return len(p), nil
}

var _ io.Writer = (*countWriter)(nil)

// latestPerFolder is the newest backup of each folder in the destination:
// one per folder ID, and one per one-off path and computer. Snapshots
// without ctdev's path tag, the markers among them, aren't backups.
func (d *Destination) latestPerFolder(ctx context.Context) ([]Snapshot, error) {
	out, err := d.restic(ctx, d.asComputer("", "snapshots", "--json", "--retry-lock", retryLock))
	if err != nil {
		return nil, err
	}
	var raw []struct {
		ID       string    `json:"id"`
		Time     time.Time `json:"time"`
		Hostname string    `json:"hostname"`
		Tags     []string  `json:"tags"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("read restic's snapshot list: %w", err)
	}
	type key struct{ id, path, host string }
	latest := map[key]Snapshot{}
	for _, r := range raw {
		s := Snapshot{ID: r.ID, Time: r.Time, Host: r.Hostname}
		for _, t := range r.Tags {
			switch {
			case strings.HasPrefix(t, tagFolder):
				s.FolderID = strings.TrimPrefix(t, tagFolder)
			case strings.HasPrefix(t, tagPath):
				s.Path = untagValue(strings.TrimPrefix(t, tagPath))
			}
		}
		if s.Path == "" {
			continue
		}
		k := key{id: s.FolderID}
		if s.FolderID == "" {
			k = key{path: s.Path, host: s.Host}
		}
		if prev, ok := latest[k]; !ok || s.Time.After(prev.Time) {
			latest[k] = s
		}
	}
	snaps := make([]Snapshot, 0, len(latest))
	for _, s := range latest {
		snaps = append(snaps, s)
	}
	slices.SortFunc(snaps, func(a, b Snapshot) int { return cmp.Compare(a.ID, b.ID) })
	return snaps, nil
}

type fileSize struct {
	path string
	size int64
}

// fileSizes lists the regular files in a backup with their recorded sizes.
func (d *Destination) fileSizes(ctx context.Context, id string) ([]fileSize, error) {
	out, err := d.restic(ctx, d.asComputer("", "ls", "--json", "--retry-lock", retryLock, id))
	if err != nil {
		return nil, err
	}
	var files []fileSize
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		var n struct {
			Type     string `json:"message_type"`
			NodeType string `json:"type"`
			Path     string `json:"path"`
			Size     int64  `json:"size"`
		}
		if json.Unmarshal(sc.Bytes(), &n) == nil && n.Type == "node" && n.NodeType == "file" {
			files = append(files, fileSize{path: n.Path, size: n.Size})
		}
	}
	slices.SortFunc(files, func(a, b fileSize) int { return cmp.Compare(a.path, b.path) })
	return files, nil
}

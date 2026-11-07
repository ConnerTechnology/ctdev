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
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
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
	// as by a computer that slept or crashed mid-run. A maintenance still
	// running after 24 hours, such as a check reading a slow backend, lets
	// another computer start one too; restic's exclusive lock then makes
	// that one's prune and check wait, or find the destination busy.
	claimTTL = 24 * time.Hour
)

// The steps of maintenance a failed DataCheck can name. The forget step
// before them isn't one: a folder's forget failure is that folder's.
const (
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

// markers lists the destination's maintenance markers, oldest first. A
// marker whose record can't be read, such as one a newer ctdev wrote, is
// left out and its ID returned in unreadable, rather than stopping every
// run for good; a finished run removes it.
func (d *Destination) markers(ctx context.Context) (ms []marker, unreadable []string, err error) {
	out, err := d.restic(ctx, d.asComputer("", "snapshots", "--json", "--retry-lock", retryLock, "--tag", tagMarker))
	if err != nil {
		return nil, nil, err
	}
	var raw []struct {
		ID   string   `json:"id"`
		Tags []string `json:"tags"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, nil, fmt.Errorf("read restic's snapshot list: %w", err)
	}
	for _, r := range raw {
		read := false
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
			if err == nil && !c.Started.IsZero() {
				ms, read = append(ms, marker{ID: r.ID, DataCheck: c}), true
			}
			break
		}
		if !read {
			unreadable = append(unreadable, r.ID)
		}
	}
	slices.SortFunc(ms, func(a, b marker) int {
		return cmp.Or(a.Started.Compare(b.Started), cmp.Compare(a.ID, b.ID))
	})
	return ms, unreadable, nil
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
// and haven't finished or been abandoned, earliest first. This computer's
// own claims, by snapshot ID, are left out: it runs one maintenance at a
// time, under its run lock, so an older claim of its own was left by a run
// that stopped, such as one whose claim couldn't be removed. Going by ID
// rather than hostname keeps two computers with the same name apart.
func activeClaims(ms []marker, now time.Time, own []string) []marker {
	last := lastResult(ms)
	var claims []marker
	for _, mk := range ms {
		if !mk.Finished.IsZero() || now.Sub(mk.Started) >= claimTTL || slices.Contains(own, mk.ID) {
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
// run started 7 or more days ago, or there's none, and no other computer is
// running one.
func maintenanceDue(ms []marker, now time.Time, own []string) bool {
	if last := lastResult(ms); last != nil && now.Sub(last.Started) < maintenanceInterval {
		return false
	}
	return len(activeClaims(ms, now, own)) == 0
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
		// Cut on a rune boundary, so the record stays valid UTF-8.
		n := maxMarkerError
		for n > 0 && !utf8.RuneStart(c.Error[n]) {
			n--
		}
		c.Error = c.Error[:n] + "…"
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
	ms, _, err := d.markers(ctx)
	if err != nil {
		return nil, err
	}
	return resultOf(ms), nil
}

// NextMaintenance is when the destination's weekly maintenance, which frees
// deleted backups' space, is next due: a week after the last one started,
// or now when that has passed or none has run.
func (d *Destination) NextMaintenance(ctx context.Context) (time.Time, error) {
	ms, _, err := d.markers(ctx)
	if err != nil {
		return time.Time{}, err
	}
	now := d.m.now()
	if last := lastResult(ms); last != nil && last.Started.Add(maintenanceInterval).After(now) {
		return last.Started.Add(maintenanceInterval), nil
	}
	return now, nil
}

// Maintenance is what one scheduled run found, and did, about a
// destination's weekly maintenance.
type Maintenance struct {
	Destination string
	Ran         bool       // this computer ran it
	Check       *DataCheck // the newest result, nil when none has run
	// Busy means this computer started it but couldn't lock the
	// destination for prune or check, so it recorded nothing and retries
	// next hour.
	Busy bool
	// RemovedUnreadable counts markers that couldn't be read, which this
	// run's cleanup removed.
	RemovedUnreadable int
	Err               error // the run failed, or the marker couldn't be read or written
}

// maintainIfDue runs d's weekly maintenance when it's due and this computer
// wins the claim. folders are this computer's scheduled folders on d, whose
// keep rules the forget step applies.
func (m *Machine) maintainIfDue(ctx context.Context, d *Destination, folders []listedFolder) (res Maintenance) {
	res.Destination = d.Name
	defer func() {
		// Before the run starts, a marker read or write, or the claim's
		// removal, that found the destination locked is the same as a busy
		// prune or check: nothing to report, retry next hour. Once it has
		// run, the result stands, and a locked destination while recording
		// it is an error like any other.
		if !res.Ran && errors.Is(res.Err, ErrDestinationBusy) {
			res.Busy, res.Err = true, nil
		}
	}()
	ms, _, err := d.markers(ctx)
	if err != nil {
		res.Err = err
		return res
	}
	res.Check = resultOf(ms)
	states, err := m.destinationStates()
	if err != nil {
		res.Err = err
		return res
	}
	own := states[d.Name].Claims
	if !maintenanceDue(ms, m.now(), own) {
		return res
	}
	// After a lost race this computer waits a random 0 to 2 hours more, so
	// two computers that keep finding it due at the same hour drift apart.
	if m.now().Before(states[d.Name].RetryAfter) {
		return res
	}
	claim := DataCheck{Host: m.Hostname, Started: m.now(), Slice: nextSlice(ms)}
	claimID, err := d.writeMarker(ctx, claim)
	if err != nil {
		res.Err = err
		return res
	}
	// This computer's claims are known by ID, not hostname, until removed.
	own = append(own, claimID)
	if err := m.setOwnClaims(d.Name, own); err != nil {
		res.Err = err
		return res
	}
	if m.afterClaim != nil {
		m.afterClaim()
	}
	// Another computer may have found it due at the same time. Every
	// computer sees the same markers: whenever there's an active claim other
	// than this one, every claimant backs off and retries later
	// (coordinator, CTD-122 review), so no two run at once, and the 7-day
	// window leaves days of hourly runs to catch up in.
	ms, unreadable, err := d.markers(ctx)
	if err != nil {
		res.Err = err
		return res
	}
	contended := len(activeClaims(ms, m.now(), own)) > 0
	if last := lastResult(ms); (last != nil && m.now().Sub(last.Started) < maintenanceInterval) || contended {
		res.Check = resultOf(ms)
		res.Err = m.dropClaim(ctx, d, claimID, own)
		if contended {
			wait := m.jitter()
			if err := m.updateDestination(d.Name, func(s *DestinationState) { s.RetryAfter = m.now().Add(wait) }); err != nil {
				res.Err = errors.Join(res.Err, err)
			}
		}
		return res
	}

	result, busy := m.maintain(ctx, d, folders, claim)
	if busy {
		// Couldn't lock the destination for prune or check within 30
		// minutes: another computer's backup or maintenance holds it. That
		// isn't a failed data check (coordinator, CTD-122 review): nothing
		// is recorded or notified, and the next hour tries again.
		res.Busy = true
		res.Err = m.dropClaim(ctx, d, claimID, own)
		return res
	}
	res.Ran, res.Check = true, &result
	var errs []error
	if result.Failed() {
		errs = append(errs, errors.New("data check "+result.Reason()))
	}
	if _, err := d.writeMarker(ctx, result); err != nil {
		res.Err = errors.Join(append(errs, err)...)
		return res
	}
	// The result replaces the markers before it, this run's claim included,
	// and any marker that can't be read: they carry ctdev's maintenance tag,
	// so they're ctdev's to remove. A later claim is another computer's,
	// which removes its own.
	old := slices.Clone(unreadable)
	for _, mk := range ms {
		if !mk.Started.After(claim.Started) {
			old = append(old, mk.ID)
		}
	}
	if len(old) > 0 {
		if err := d.forget(ctx, old...); err != nil {
			errs = append(errs, fmt.Errorf("remove the old maintenance markers: %w", err))
		} else {
			res.RemovedUnreadable = len(unreadable)
			// Every claim of this computer's started no later than this one.
			errs = append(errs, m.setOwnClaims(d.Name, nil))
		}
	}
	res.Err = errors.Join(errs...)
	return res
}

// dropClaim removes this computer's claim id, and forgets it as its own
// once it's gone; one that can't be removed stays its own, so it never
// counts as another computer's.
func (m *Machine) dropClaim(ctx context.Context, d *Destination, id string, own []string) error {
	if err := d.forget(ctx, id); err != nil {
		return fmt.Errorf("remove this computer's claim on the maintenance: %w", err)
	}
	return m.setOwnClaims(d.Name, slices.DeleteFunc(own, func(c string) bool { return c == id }))
}

// setOwnClaims records which markers in the destination are this
// computer's claims.
func (m *Machine) setOwnClaims(name string, ids []string) error {
	return m.updateDestination(name, func(s *DestinationState) { s.Claims = ids })
}

// resultOf is a copy of the newest finished run among ms, or nil.
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

// maintain runs the four steps in order. The data check's result is prune,
// check and test download's, stopping at the first that fails. busy means
// prune or check couldn't lock the destination, and there's no result.
func (m *Machine) maintain(ctx context.Context, d *Destination, folders []listedFolder, claim DataCheck) (result DataCheck, busy bool) {
	c := claim
	fail := func(step string, err error) (DataCheck, bool) {
		c.Finished, c.Step, c.Error = m.now(), step, strings.ReplaceAll(err.Error(), "\n", "; ")
		return c, false
	}
	// 1. The keep rules of this computer's folders, again: a safety net for
	// a run whose retention failed, which this clears when it succeeds. A
	// failure is that folder's, recorded on it as its yellow retention
	// error by Retain, and isn't the data check's (coordinator, CTD-122
	// review): maintenance goes on.
	for _, f := range folders {
		_ = m.Retain(ctx, d, f.Root, f.Config)
	}
	// 2. Prune frees the space of every forgotten backup. It takes the
	// exclusive lock, so backups wait for it (--retry-lock).
	if _, err := d.restic(ctx, d.asComputer("", "prune", "--retry-lock", retryLock)); errors.Is(err, ErrDestinationBusy) {
		return DataCheck{}, true
	} else if err != nil {
		return fail(StepPrune, err)
	}
	// 3. Read this week's twentieth of the stored data.
	subset := "--read-data-subset=" + strconv.Itoa(c.Slice) + "/" + strconv.Itoa(Slices)
	check := d.asComputer("", "check", "--retry-lock", retryLock, subset)
	var stderr bytes.Buffer
	check.stderr = &stderr
	if _, err := d.restic(ctx, check); errors.Is(err, ErrDestinationBusy) {
		return DataCheck{}, true
	} else if err != nil {
		return fail(StepCheck, checkFailure(stderr.Bytes(), err))
	}
	// 4. A small test restore: one random file from each folder's latest
	// backup reads back whole.
	if err := m.testDownload(ctx, d); err != nil {
		return fail(StepDownload, err)
	}
	c.Finished, c.OK = m.now(), true
	return c, false
}

// jitter is how much longer than an hour this computer waits after losing a
// race for the maintenance: 0, 1 or 2 hours.
func (m *Machine) jitter() time.Duration {
	if m.jitterFn != nil {
		return m.jitterFn()
	}
	return time.Duration(rand.IntN(3)) * time.Hour
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
		f := m.pick(files)
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

// pick chooses the file to test-download, at random.
func (m *Machine) pick(files []fileSize) fileSize {
	if m.pickFn != nil {
		paths := make([]string, len(files))
		for i, f := range files {
			paths[i] = f.path
		}
		return files[m.pickFn(paths)]
	}
	return files[rand.IntN(len(files))]
}

type countWriter int64

func (w *countWriter) Write(p []byte) (int, error) {
	*w += countWriter(len(p))
	return len(p), nil
}

// latestPerFolder is the newest backup of each folder in the destination:
// one per folder ID, and one per one-off path and computer. Snapshots
// without ctdev's path tag, the markers among them, aren't backups.
func (d *Destination) latestPerFolder(ctx context.Context) ([]Snapshot, error) {
	all, err := d.snapshots(ctx, "snapshots", "--json", "--retry-lock", retryLock)
	if err != nil {
		return nil, err
	}
	type key struct{ id, path, host string }
	latest := map[key]Snapshot{}
	for _, s := range all {
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

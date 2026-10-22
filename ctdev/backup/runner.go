package backup

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// Outcome is what a scheduled run did with one folder.
type Outcome string

const (
	BackedUp Outcome = "backed up"
	NotDue   Outcome = "not due"
	Missing  Outcome = "missing"
	Failed   Outcome = "failed"
	// Deferred means the destination was locked, usually by another
	// computer's weekly maintenance, so the backup waits for the next hour.
	Deferred Outcome = "deferred"
)

// ScheduledRun is one listed folder's result from RunScheduled.
type ScheduledRun struct {
	Path     string
	Outcome  Outcome
	Err      error    // why it failed or is missing
	Snapshot Snapshot // the backup made, when BackedUp
	// RetentionErr is why the keep rules didn't apply after the backup. The
	// backup still succeeded, so it isn't Err.
	RetentionErr error
}

// RunScheduled is what the hourly runner does: back up each folder on this
// computer's list whose schedule is due, the same way Back up now does,
// recording each result in state.json, then notify about any folder that
// started failing or recovered. A listed folder that's gone, or whose config
// file is gone or holds another folder's ID, is skipped and recorded as
// missing. The error joins every folder's failure, so the run itself shows as
// failed in the journal or log.
//
// Then, for each destination those folders use, it reads the weekly
// maintenance's marker in the destination, keeps a copy of the result in
// state.json for ctdev doctor, and runs the maintenance when it's due
// (maintainIfDue). A data check that failed notifies once, as a failing
// folder does.
//
// It holds this computer's backup lock for the whole run. When another run
// holds it, this one returns nothing and no error: that run covers the hour.
// A dry run writes nothing at all, the lock file included, and leaves the
// maintenance alone.
func (m *Machine) RunScheduled(ctx context.Context) ([]ScheduledRun, []Maintenance, error) {
	if !m.Opts.DryRun {
		unlock, err := m.runLock()
		if errors.Is(err, errLocked) {
			return nil, nil, nil
		}
		if err != nil {
			return nil, nil, err
		}
		defer unlock()
	}
	folders, err := m.scheduledFolders()
	if err != nil {
		return nil, nil, err
	}
	runs, err := m.runs()
	if err != nil {
		return nil, nil, err
	}
	var results []ScheduledRun
	var errs []error
	for _, e := range folders {
		if ctx.Err() != nil {
			break
		}
		r := m.runOne(ctx, e, runs[e.Path])
		if r.Err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", DisplayPath(m.Home, e.Path), r.Err))
		}
		results = append(results, r)
	}
	if m.Opts.DryRun {
		return results, nil, errors.Join(errs...)
	}
	maint := m.maintainDestinations(ctx, folders)
	for _, r := range maint {
		if r.Err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", r.Destination, r.Err))
		}
	}
	errs = append(errs, m.notifyChanges(ctx, folders))
	return results, maint, errors.Join(errs...)
}

// maintainDestinations does the weekly maintenance part of a scheduled run
// for every destination a listed folder uses, recording each result in
// state.json. A folder that's missing, or whose destination isn't set up
// here, already failed its backup and is left out.
func (m *Machine) maintainDestinations(ctx context.Context, folders []folderEntry) []Maintenance {
	byDest := map[string][]listedFolder{}
	for _, e := range folders {
		if cfg, why := m.findListed(e); why == "" {
			byDest[cfg.Destination] = append(byDest[cfg.Destination], listedFolder{Root: e.Path, Config: cfg})
		}
	}
	var out []Maintenance
	for _, name := range sortedKeys(byDest) {
		if ctx.Err() != nil {
			break
		}
		d, err := m.Destination(name)
		if err != nil {
			continue
		}
		r := m.maintainIfDue(ctx, d, byDest[name])
		if r.Check != nil {
			if err := m.recordDataCheck(name, *r.Check); err != nil {
				r.Err = errors.Join(r.Err, err)
			}
		}
		out = append(out, r)
	}
	return out
}

func (m *Machine) runOne(ctx context.Context, e folderEntry, last Run) ScheduledRun {
	res := ScheduledRun{Path: e.Path}
	cfg, why := m.findListed(e)
	dry := m.Opts.DryRun
	if why != "" {
		res.Outcome, res.Err = Missing, errors.New(why)
		if dry {
			return res
		}
		if err := m.recordMissing(e.Path, m.now(), why); err != nil {
			res.Err = errors.Join(res.Err, err)
		}
		return res
	}
	if dry {
		// Say what would happen, writing nothing: Backup dry-runs too.
		if !Due(cfg.Schedule, last, m.now()) {
			res.Outcome = NotDue
			return res
		}
		d, err := m.Destination(cfg.Destination)
		if err != nil {
			res.Outcome, res.Err = Failed, err
			return res
		}
		if _, err := m.Backup(ctx, d, e.Path, &cfg); err != nil {
			res.Outcome, res.Err = Failed, err
			return res
		}
		res.Outcome, res.RetentionErr = BackedUp, m.Retain(ctx, d, e.Path, cfg)
		return res
	}
	if last.Missing {
		// It's back. Its last success still decides whether it's due.
		if err := m.updateRun(e.Path, func(r *Run) { r.Missing, r.Error = false, "" }); err != nil {
			res.Outcome, res.Err = Failed, err
			return res
		}
		last.Missing, last.Error = false, ""
	}
	if err := m.setListed(e.Path, cfg); err != nil {
		res.Outcome, res.Err = Failed, err
		return res
	}
	if !Due(cfg.Schedule, last, m.now()) {
		res.Outcome = NotDue
		return res
	}
	d, err := m.Destination(cfg.Destination)
	if err != nil {
		res.Outcome, res.Err = Failed, err
		if rerr := m.recordRun(e.Path, m.now(), err); rerr != nil {
			res.Err = errors.Join(err, rerr)
		}
		return res
	}
	// Backup records the result in state.json itself.
	res.Snapshot, err = m.Backup(ctx, d, e.Path, &cfg)
	if errors.Is(err, ErrDestinationBusy) {
		// Not a failure (coordinator, CTD-122 review): Backup recorded it as
		// deferred, nothing notifies, and the next hour tries again. A
		// destination busy for long enough still turns overdue.
		res.Outcome = Deferred
		return res
	}
	if err != nil {
		res.Outcome, res.Err = Failed, err
		return res
	}
	res.Outcome = BackedUp
	// The folder's keep rules apply after each successful backup, to its own
	// backups only (CTD-87). A failure doesn't undo the backup, which stays
	// the last success: it's recorded apart, as Back up now records it.
	res.RetentionErr = m.Retain(ctx, d, e.Path, cfg)
	return res
}

// findListed reads the config of a listed folder, or says why it's missing.
func (m *Machine) findListed(e folderEntry) (Config, string) {
	if fi, err := os.Stat(e.Path); err != nil || !fi.IsDir() {
		return Config{}, "the folder isn't there any more: move it back, or run ctdev backup in its new place and choose Configure"
	}
	cfg, err := ReadConfig(e.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, "its " + ConfigFileName + " is gone: run ctdev backup there and choose Configure"
	}
	if err != nil {
		return Config{}, err.Error()
	}
	if cfg.ID != e.ID {
		return Config{}, "its " + ConfigFileName + " belongs to another folder now: run ctdev backup there and choose Configure"
	}
	return cfg, ""
}

// notifyChanges sends one notification when a listed folder starts failing
// (a failed or missing last attempt) and one when it recovers, never one per
// failed run. Whether the "failing" one went out is kept in state.json, so a
// success from Back up now counts as a recovery too.
func (m *Machine) notifyChanges(ctx context.Context, folders []folderEntry) error {
	runs, err := m.runs()
	if err != nil {
		return err
	}
	n := m.notifier()
	for _, e := range folders {
		r := runs[e.Path]
		failing := r.Failed()
		if failing == r.Alerted {
			continue
		}
		where := DisplayPath(m.Home, e.Path)
		if failing {
			_ = n.Notify(ctx, "Backup failing", where+": "+r.Error)
		} else {
			_ = n.Notify(ctx, "Backup working again", where+" is backing up again.")
		}
		// A notification that couldn't be shown isn't retried: there's no
		// desktop to show it on, and ctdev backup and doctor still say so.
		if err := m.updateRun(e.Path, func(r *Run) { r.Alerted = failing }); err != nil {
			return err
		}
	}
	// The same for each destination's data check, which every computer
	// sees, so each notifies its own desktop once.
	dests, err := m.destinationStates()
	if err != nil {
		return err
	}
	for _, name := range sortedKeys(dests) {
		s := dests[name]
		failing := s.DataCheck.Failed()
		if failing == s.Alerted {
			continue
		}
		if failing {
			_ = n.Notify(ctx, "Backup data check failed", name+": "+s.DataCheck.Reason())
		} else {
			_ = n.Notify(ctx, "Backup data check passing again", name+": the data check passed.")
		}
		if err := m.updateDestination(name, func(s *DestinationState) { s.Alerted = failing }); err != nil {
			return err
		}
	}
	return nil
}

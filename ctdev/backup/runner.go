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
)

// ScheduledRun is one listed folder's result from RunScheduled.
type ScheduledRun struct {
	Path     string
	Outcome  Outcome
	Err      error    // why it failed or is missing
	Snapshot Snapshot // the backup made, when BackedUp
}

// RunScheduled is what the hourly runner does: back up each folder on this
// computer's list whose schedule is due, the same way Back up now does,
// recording each result in state.json, then notify about any folder that
// started failing or recovered. A listed folder that's gone, or whose config
// file is gone or holds another folder's ID, is skipped and recorded as
// missing. The error joins every folder's failure, so the run itself shows as
// failed in the journal or log.
//
// It holds this computer's backup lock for the whole run. When another run
// holds it, this one returns nothing and no error: that run covers the hour.
// A dry run writes nothing at all, the lock file included.
func (m *Machine) RunScheduled(ctx context.Context) ([]ScheduledRun, error) {
	if !m.Opts.DryRun {
		unlock, err := m.lock(false)
		if errors.Is(err, errLocked) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		defer unlock()
	}
	folders, err := m.scheduledFolders()
	if err != nil {
		return nil, err
	}
	runs, err := m.runs()
	if err != nil {
		return nil, err
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
	if !m.Opts.DryRun {
		errs = append(errs, m.notifyChanges(ctx, folders))
	}
	return results, errors.Join(errs...)
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
		res.Outcome = BackedUp
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
	if err := m.setSchedule(e.Path, cfg.Schedule); err != nil {
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
	if err != nil {
		res.Outcome, res.Err = Failed, err
		return res
	}
	res.Outcome = BackedUp
	// CTD-120: apply the folder's retention (cfg.Keep) here, after each
	// successful backup and to this folder's backups only, once that ticket
	// adds the package function for it.
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
	return nil
}

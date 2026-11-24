package backupapp

import (
	"context"
	"errors"

	"github.com/ConnerTechnology/ctdev/ctdev/backup"
)

// listBackups is List backups: the folder's backups, one backup's details,
// download and delete (rows V).
func listBackups(ctx context.Context, ui UI, d Deps, s backup.Status) error {
	return errors.New("List backups isn't built yet")
}

// stopBackingUp is Stop backing up (rows Y).
func stopBackingUp(ctx context.Context, ui UI, d Deps, s backup.Status) error {
	return errors.New("Stop backing up isn't built yet")
}

// deleteAllBackups is Delete all backups (rows Y).
func deleteAllBackups(ctx context.Context, ui UI, d Deps, s backup.Status) error {
	return errors.New("Delete all isn't built yet")
}

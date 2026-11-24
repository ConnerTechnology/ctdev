package backupapp

import (
	"context"
	"errors"

	"github.com/ConnerTechnology/ctdev/ctdev/backup"
)

// browseBackups is a folder's backups from List backups (rows V), under
// --all too. A stub until the List backups slice lands.
func browseBackups(ctx context.Context, ui UI, d Deps, dests []*backup.Destination, ref backup.Ref, where string,
	download func(ctx context.Context, dest *backup.Destination, snap backup.Snapshot) (string, error)) error {
	return errors.New("browsing backups isn't built yet")
}

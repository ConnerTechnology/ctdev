package backupapp

import (
	"context"
	"errors"

	"github.com/ConnerTechnology/ctdev/ctdev/backup"
)

// configureFolder is Configure (rows W and X).
func configureFolder(ctx context.Context, ui UI, d Deps, s backup.Status) error {
	return errors.New("Configure isn't built yet")
}

// createDestinationForOnce sets up this computer's first destination for a
// one-off backup (U1 → W5), and returns it.
func createDestinationForOnce(ctx context.Context, ui UI, d Deps, s backup.Status) (*backup.Destination, error) {
	return nil, errors.New("setting up a destination isn't built yet")
}

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

// createDestination is the new-destination step (W5 to W10): name, backend,
// credentials, master password, creating it, and the kit shown once. check
// refuses an address (CheckNewAddress, or CheckNewAddressFromAll under --all).
func createDestination(ctx context.Context, ui UI, d Deps, preferred string, check func(string) (string, error)) (*backup.Destination, error) {
	return nil, errors.New("creating a destination isn't built yet")
}

// connectDestination connects this computer with an Emergency Kit (X1 to X11).
func connectDestination(ctx context.Context, ui UI, d Deps) (*backup.Destination, error) {
	return nil, errors.New("connecting a destination isn't built yet")
}

// recoveryReset sets a new master password and Secret Key with a Recovery
// Code (X7 to X10, P11 to P13), and shows the new kit. recover is
// Destination.Recover or a RecoverFromKit closure.
func recoveryReset(ctx context.Context, ui UI, d Deps, name string, recover func(ctx context.Context, code, master string, newCode bool) (backup.Recovery, error)) error {
	return errors.New("the Recovery Code reset isn't built yet")
}

// pasteUpdatedKit reads an Emergency Kit and updates this computer from it
// (X4, P14 to P16), and returns the destination it updated.
func pasteUpdatedKit(ctx context.Context, ui UI, d Deps) (*backup.Destination, error) {
	return nil, errors.New("pasting a kit isn't built yet")
}

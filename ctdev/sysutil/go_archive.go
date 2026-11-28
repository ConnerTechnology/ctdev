package sysutil

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// GoInstallDir is the official Go archive's installation directory.
const GoInstallDir = "/usr/local/go"

// RecoverGoArchive finishes an interrupted swap before either entry point
// needs the network again. Without a leftover transaction this is read-only.
func RecoverGoArchive(ctx context.Context, o Opts) error {
	if o.DryRun {
		return nil
	}
	if _, err := os.Lstat(GoInstallDir + ".ctdev-install"); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	unlock, err := lockDeployDir(filepath.Dir(GoInstallDir), false)
	if err != nil {
		return err
	}
	defer unlock()
	return SudoRun(ctx, o, "sh", "-ec", recoverGoArchive, "ctdev", GoInstallDir)
}

// InstallGoArchive downloads an official archive and activates it on the
// same filesystem as the current Go tree. A failed activation puts the old
// tree back. A later install can also recover it after a power loss.
func InstallGoArchive(ctx context.Context, o Opts, url, sha string) error {
	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] download %s, verify sha256 and install to %s\n", url, GoInstallDir)
		return nil
	}
	dir, err := os.MkdirTemp("", "ctdev-go-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	archive := filepath.Join(dir, "go.tar.gz")
	if err := DownloadFile(ctx, url, archive); err != nil {
		return err
	}
	if err := VerifyChecksum(archive, sha); err != nil {
		return fmt.Errorf("verify Go archive: %w", err)
	}

	unlock, err := lockDeployDir(filepath.Dir(GoInstallDir), false)
	if err != nil {
		return err
	}
	defer unlock()
	err = SudoRun(ctx, o, "sh", "-ec", replaceGoArchive, "ctdev", GoInstallDir, archive, sha)
	if err != nil && ctx.Err() != nil {
		// Run kills the entire shell process group on cancellation. Complete
		// its interrupted rollback while we still hold the deployment lock.
		recoverErr := SudoRun(context.WithoutCancel(ctx), o, "sh", "-ec", recoverGoArchive, "ctdev", GoInstallDir)
		return errors.Join(err, recoverErr)
	}
	return err
}

// The transaction directory is root-owned, next to the live tree. Recheck
// the copied archive inside it before privileged extraction: a user-owned
// download outside it can change after Go verified it. Keep "previous"
// until activation succeeds, including if the shell is killed between moves.
const recoverGoArchive = `
live=$1
stage=${live}.ctdev-install
previous=$stage/previous
marker=$stage/ctdev-go-transaction-v1
exists() { [ -e "$1" ] || [ -L "$1" ]; }
cleanup() {
	if exists "$previous" && ! exists "$live"; then
		mv "$previous" "$live" || { echo "previous Go installation is at $previous" >&2; return 1; }
	fi
	rm -rf "$stage/go" "$previous"
	rm -f "$stage/archive" "$marker"
	rmdir "$stage"
}
if exists "$stage"; then
	if [ -L "$stage" ] || [ ! -f "$marker" ] || [ -L "$marker" ]; then
		# A crash between mkdir and marking it can leave an empty directory.
		rmdir "$stage" || { echo "unrecognized Go staging directory $stage" >&2; exit 1; }
	else
		cleanup
	fi
fi
`

const replaceGoArchive = recoverGoArchive + `
archive=$2
sha=$3
mkdir -m 700 "$stage"
: > "$marker"
trap cleanup EXIT
trap 'exit 1' HUP INT TERM
cp "$archive" "$stage/archive"
if command -v sha256sum >/dev/null 2>&1; then
	actual=$(sha256sum < "$stage/archive")
else
	actual=$(shasum -a 256 < "$stage/archive")
fi
[ "${actual%% *}" = "$sha" ] || { echo "Go archive checksum mismatch" >&2; exit 1; }
tar --no-same-owner -C "$stage" -xzf "$stage/archive"
[ -f "$stage/go/bin/go" ] && [ -x "$stage/go/bin/go" ] || { echo "Go archive missing executable bin/go" >&2; exit 1; }
if exists "$live"; then
	mv "$live" "$previous"
fi
mv "$stage/go" "$live"
`

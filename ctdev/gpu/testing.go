package gpu

import "testing"

// SetEFIDirForTest points the UEFI check in SecureBootState at dir for the
// duration of the test, so a test decides whether this "machine" booted
// through UEFI instead of inheriting the host's firmware.
func SetEFIDirForTest(t testing.TB, dir string) {
	t.Helper()
	old := efiDir
	efiDir = dir
	t.Cleanup(func() { efiDir = old })
}

package gpu

// testCleanup is the subset of testing.TB SetEFIDirForTest uses. Naming the
// interface avoids importing "testing" into the production binary.
type testCleanup interface {
	Helper()
	Cleanup(func())
}

// SetEFIDirForTest points the UEFI check in SecureBootState at dir for the
// duration of the test, so a test decides whether this "machine" booted
// through UEFI instead of inheriting the host's firmware.
func SetEFIDirForTest(t testCleanup, dir string) {
	t.Helper()
	old := efiDir
	efiDir = dir
	t.Cleanup(func() { efiDir = old })
}

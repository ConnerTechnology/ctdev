package component

import "testing"

// RegisterForTest appends c to the Registry for the duration of the test and
// restores the original Registry afterwards. Use this when a test needs
// FindByName to resolve a component that isn't part of the real registry.
func RegisterForTest(t testing.TB, c Component) {
	t.Helper()
	orig := Registry
	Registry = append(append([]Component{}, orig...), c)
	t.Cleanup(func() { Registry = orig })
}

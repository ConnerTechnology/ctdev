package component

import "testing"

// RegisterForTest appends c to the Registry for the duration of the test and
// restores the original Registry afterwards. Use this when a test needs
// FindByName to resolve a component that isn't part of the real registry.
// The componentByName index is rebuilt too — Registry and the index are two
// views of the same data, and leaving the index stale would make
// alreadyInstalled() disagree with FindByName for the test component.
func RegisterForTest(t testing.TB, c Component) {
	t.Helper()
	origRegistry := Registry
	origIndex := componentByName
	Registry = append(append([]Component{}, origRegistry...), c)
	index := make(map[string]*Component, len(Registry))
	for i := range Registry {
		index[Registry[i].Name] = &Registry[i]
	}
	componentByName = index
	t.Cleanup(func() {
		Registry = origRegistry
		componentByName = origIndex
	})
}

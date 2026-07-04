package component

import (
	"slices"
	"testing"
)

func TestDefaultBackupExcludes(t *testing.T) {
	ex := DefaultBackupExcludes()
	if !slices.Contains(ex, "*.sock") {
		t.Errorf("expected sockets excluded, got %v", ex)
	}
}

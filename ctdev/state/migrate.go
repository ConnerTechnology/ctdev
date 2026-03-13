package state

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

func MigrateOldMarkers(oldDir string, store *MarkerStore) (int, error) {
	entries, err := os.ReadDir(oldDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	migrated := 0
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".installed") {
			continue
		}
		componentName := strings.TrimSuffix(name, ".installed")

		if store.Exists(componentName) {
			continue
		}

		data, err := os.ReadFile(filepath.Join(oldDir, name))
		if err != nil {
			continue
		}

		installedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(string(data)))
		if err != nil {
			installedAt = time.Now()
		}

		marker := InstallMarker{
			InstalledAt: installedAt,
			Version:     "unknown",
			UpdatedAt:   installedAt,
		}

		if err := store.Save(componentName, marker); err != nil {
			continue
		}
		migrated++
	}
	return migrated, nil
}

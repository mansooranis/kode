package bundled

import (
	"os"
	"path/filepath"
	"strings"
)

// Sync writes every bundled skill into dir as "<name>.md", matching the flat
// layout Library.Load expects (see ../loader.go). It always overwrites —
// bundled skills are kode-managed, so local edits under these names won't
// survive an upgrade. Skills with other names in dir are left alone.
// Returns the names written.
func Sync(dir string) ([]string, error) {
	dir = expandHome(dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	skills, err := All()
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(skills))
	for _, s := range skills {
		path := filepath.Join(dir, s.Name+".md")
		if err := os.WriteFile(path, []byte(s.Body), 0o644); err != nil {
			return names, err
		}
		names = append(names, s.Name)
	}
	return names, nil
}

// EnsureSynced runs Sync against dir, tracking the last-synced version in a
// marker file alongside dir. Unless force is set, it's a no-op when version
// (normally buildinfo.Version) matches the marker, so a normal `kode`
// invocation re-syncs bundled skills exactly once per upgrade rather than on
// every startup. The marker is updated after every sync, forced or not.
func EnsureSynced(dir, version string, force bool) (synced bool, names []string, err error) {
	dir = expandHome(dir)
	marker := filepath.Join(filepath.Dir(dir), ".skills-version")

	if !force {
		if last, readErr := os.ReadFile(marker); readErr == nil && strings.TrimSpace(string(last)) == version {
			return false, nil, nil
		}
	}

	names, err = Sync(dir)
	if err != nil {
		return false, names, err
	}

	_ = os.WriteFile(marker, []byte(version), 0o644)
	return true, names, nil
}

func expandHome(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~"))
}

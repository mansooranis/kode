package bundled

import (
	"fmt"
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

// InstallClaudeSkill writes one bundled skill into dir/<name>/SKILL.md,
// matching Claude Code's own skill folder layout (as opposed to Sync's flat
// "<name>.md" layout for kode's own agent). dir is typically
// ~/.claude/skills, Claude Code's global skills folder, so a separate
// `claude` session picks the skill up without this repo's project-local
// .claude/skills/kode-comments symlink. Returns the path written to.
func InstallClaudeSkill(dir, name string) (string, error) {
	dir = expandHome(dir)

	skills, err := All()
	if err != nil {
		return "", err
	}

	for _, s := range skills {
		if s.Name != name {
			continue
		}
		skillDir := filepath.Join(dir, s.Name)
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			return "", err
		}
		path := filepath.Join(skillDir, "SKILL.md")
		if err := os.WriteFile(path, []byte(s.Body), 0o644); err != nil {
			return "", err
		}
		return path, nil
	}
	return "", fmt.Errorf("unknown bundled skill %q", name)
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

// Package update checks GitHub for a newer kode release than the running
// binary, so the TUI can show a small "update available" banner. It never
// downloads or installs anything — actually replacing the binary stays the
// job of whatever installed it (e.g. Homebrew).
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"
)

// apiURL is a var (not a const) so tests can point it at an httptest server.
var apiURL = "https://api.github.com/repos/mansooranis/kode/releases/latest"

// cacheTTL bounds how often Check actually hits the network: repeated
// invocations within this window (e.g. kode opened many times in a day)
// reuse the last result instead of a fresh GitHub API call every time.
const cacheTTL = 24 * time.Hour

const httpTimeout = 3 * time.Second

// Result is what a caller needs to decide whether to show a banner.
type Result struct {
	Latest    string // e.g. "v0.0.4"; empty if unknown
	Available bool   // true if Latest is a newer release than the version passed to Check
}

type cacheData struct {
	CheckedAt time.Time `json:"checked_at"`
	Latest    string    `json:"latest"`
}

// Check compares currentVersion against the latest GitHub release, using
// cachePath to avoid a network round-trip on every call within cacheTTL. A
// currentVersion that isn't a parseable release version (e.g. "dev", or a
// git-describe string with commits-since-tag/dirty suffixes) is treated as
// an unreleased build: Check returns a zero Result and no error rather than
// guessing at a comparison.
func Check(ctx context.Context, currentVersion, cachePath string) (Result, error) {
	if _, ok := parseSemver(currentVersion); !ok {
		return Result{}, nil
	}

	latest, err := latestFromCache(cachePath)
	if err != nil || latest == "" {
		latest, err = fetchLatest(ctx)
		if err != nil {
			return Result{}, err
		}
		writeCache(cachePath, latest)
	}

	cur, _ := parseSemver(currentVersion)
	lat, ok := parseSemver(latest)
	if !ok {
		return Result{}, nil
	}

	return Result{Latest: latest, Available: lat.newerThan(cur)}, nil
}

// latestFromCache returns a cached latest-version string if it's still
// within cacheTTL, or "" (with no error) on a miss/expired/missing cache —
// all of which just mean "go fetch a fresh one".
func latestFromCache(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil
	}
	var c cacheData
	if err := json.Unmarshal(data, &c); err != nil {
		return "", nil
	}
	if time.Since(c.CheckedAt) > cacheTTL {
		return "", nil
	}
	return c.Latest, nil
}

// writeCache is best-effort: a failure to persist the cache just means the
// next kode invocation hits the network again, which is harmless.
func writeCache(path, latest string) {
	data, err := json.Marshal(cacheData{CheckedAt: time.Now(), Latest: latest})
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o644)
}

type ghRelease struct {
	TagName string `json:"tag_name"`
}

func fetchLatest(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, httpTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "kode-update-check")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github releases API: unexpected status %s", resp.Status)
	}

	var rel ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", err
	}
	if rel.TagName == "" {
		return "", fmt.Errorf("github releases API: no tag_name in response")
	}
	return rel.TagName, nil
}

type semver struct{ major, minor, patch int }

func (a semver) newerThan(b semver) bool {
	if a.major != b.major {
		return a.major > b.major
	}
	if a.minor != b.minor {
		return a.minor > b.minor
	}
	return a.patch > b.patch
}

var semverRe = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)$`)

// parseSemver only accepts a clean "v1.2.3" (or "1.2.3") tag — deliberately
// rejecting git-describe's commits-ahead/dirty suffixes (e.g.
// "v0.0.3-2-g1a2b3c4-dirty") rather than guessing at a meaningful
// comparison for an unreleased build.
func parseSemver(s string) (semver, bool) {
	m := semverRe.FindStringSubmatch(s)
	if m == nil {
		return semver{}, false
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	patch, _ := strconv.Atoi(m[3])
	return semver{major, minor, patch}, true
}

// CachePath returns the default location Check's cache lives at, alongside
// kode's other user-level state.
func CachePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "kode", "update-check.json"), nil
}

// AvailableMsg is sent into a running Bubble Tea program when Check finds a
// newer release, for ui.App/explain.App to display as a banner.
type AvailableMsg struct {
	Latest string
}

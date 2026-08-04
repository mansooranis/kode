package update

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func withServer(t *testing.T, tagName string) func() {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(ghRelease{TagName: tagName})
	}))
	orig := apiURL
	apiURL = srv.URL
	return func() {
		srv.Close()
		apiURL = orig
	}
}

func TestCheckReportsNewerRelease(t *testing.T) {
	defer withServer(t, "v0.2.0")()

	cache := filepath.Join(t.TempDir(), "update-check.json")
	result, err := Check(context.Background(), "v0.1.0", cache)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !result.Available || result.Latest != "v0.2.0" {
		t.Fatalf("got %+v, want available v0.2.0", result)
	}
}

func TestCheckNoUpdateWhenCurrent(t *testing.T) {
	defer withServer(t, "v0.1.0")()

	cache := filepath.Join(t.TempDir(), "update-check.json")
	result, err := Check(context.Background(), "v0.1.0", cache)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.Available {
		t.Fatalf("got %+v, want not available", result)
	}
}

func TestCheckSkipsUnreleasedBuilds(t *testing.T) {
	defer withServer(t, "v99.0.0")()

	cache := filepath.Join(t.TempDir(), "update-check.json")
	for _, v := range []string{"dev", "v0.1.0-3-g1a2b3c4", "v0.1.0-dirty"} {
		result, err := Check(context.Background(), v, cache)
		if err != nil {
			t.Fatalf("Check(%q): %v", v, err)
		}
		if result.Available {
			t.Fatalf("Check(%q) = %+v, want no comparison attempted", v, result)
		}
	}
}

func TestCheckUsesCacheWithinTTL(t *testing.T) {
	closeServer := withServer(t, "v0.2.0")
	cache := filepath.Join(t.TempDir(), "update-check.json")

	if _, err := Check(context.Background(), "v0.1.0", cache); err != nil {
		t.Fatalf("Check: %v", err)
	}
	closeServer() // network now fails; a cache hit must not need it

	result, err := Check(context.Background(), "v0.1.0", cache)
	if err != nil {
		t.Fatalf("Check (cached): %v", err)
	}
	if !result.Available || result.Latest != "v0.2.0" {
		t.Fatalf("got %+v, want cached available v0.2.0", result)
	}
}

func TestCheckRefetchesAfterTTLExpires(t *testing.T) {
	defer withServer(t, "v0.3.0")()

	cache := filepath.Join(t.TempDir(), "update-check.json")
	stale := cacheData{CheckedAt: time.Now().Add(-25 * time.Hour), Latest: "v0.2.0"}
	data, _ := json.Marshal(stale)
	if err := os.WriteFile(cache, data, 0o644); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	result, err := Check(context.Background(), "v0.1.0", cache)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.Latest != "v0.3.0" {
		t.Fatalf("got %+v, want a refreshed v0.3.0", result)
	}
}

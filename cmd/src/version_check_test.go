package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sourcegraph/src-cli/internal/version"
)

// setBuildTag pretends that src-cli was built as the given release.
func setBuildTag(t *testing.T, buildTag string) {
	t.Helper()
	original := version.BuildTag
	t.Cleanup(func() { version.BuildTag = original })
	version.BuildTag = buildTag
}

// versionServer is a fake Sourcegraph instance that answers every request with
// the given status and recommended version, and counts the requests it gets.
func versionServer(t *testing.T, status int, recommended string) (cfg *config, requests *int) {
	t.Helper()
	requests = new(int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*requests++
		if r.URL.Path != "/.api/src-cli/version" {
			t.Errorf("unexpected request path %q", r.URL.Path)
		}
		w.WriteHeader(status)
		fmt.Fprintf(w, `{"version": %q}`, recommended)
	}))
	t.Cleanup(server.Close)
	return &config{endpointURL: mustParseURL(t, server.URL), accessToken: "token"}, requests
}

func TestOutdatedVersionWarning(t *testing.T) {
	tests := []struct {
		name        string
		current     string
		status      int
		recommended string
		wantWarning bool
	}{
		{name: "older patch", current: "6.4.1", status: http.StatusOK, recommended: "6.4.2", wantWarning: true},
		{name: "older, compared as numbers not strings", current: "9.9.9", status: http.StatusOK, recommended: "10.0.0", wantWarning: true},
		{name: "newer, compared as numbers not strings", current: "10.0.0", status: http.StatusOK, recommended: "9.9.9"},
		{name: "same version", current: "6.4.2", status: http.StatusOK, recommended: "6.4.2"},
		{name: "instance does not recommend a version", current: "6.4.2", status: http.StatusNotFound},
		{name: "instance returns an error", current: "6.4.2", status: http.StatusInternalServerError, recommended: "7.0.0"},
		{name: "instance returns an unparseable version", current: "6.4.2", status: http.StatusOK, recommended: "latest"},
		{name: "running an unparseable version", current: "custom-build", status: http.StatusOK, recommended: "7.0.0"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setBuildTag(t, test.current)
			cfg, _ := versionServer(t, test.status, test.recommended)

			warning := outdatedVersionWarning(cfg, filepath.Join(t.TempDir(), "recommended-version"))

			if !test.wantWarning {
				if warning != "" {
					t.Fatalf("got warning %q, want none", warning)
				}
				return
			}
			for _, want := range []string{test.current, test.recommended, cfg.endpointURL.String(), "SRC_SKIP_VERSION_CHECK"} {
				if !strings.Contains(warning, want) {
					t.Errorf("warning %q does not mention %q", warning, want)
				}
			}
		})
	}
}

func TestRecommendedVersionIsLookedUpOncePerInterval(t *testing.T) {
	makeStale := func(t *testing.T, cachePath string) {
		t.Helper()
		stale := time.Now().Add(-versionCheckInterval - time.Minute)
		if err := os.Chtimes(cachePath, stale, stale); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("answer is reused until it is stale", func(t *testing.T) {
		setBuildTag(t, "6.4.1")
		cfg, requests := versionServer(t, http.StatusOK, "6.4.2")
		cachePath := filepath.Join(t.TempDir(), "nested", "recommended-version")

		for range 2 {
			if warning := outdatedVersionWarning(cfg, cachePath); warning == "" {
				t.Fatal("got no warning, want one on every command")
			}
		}
		if *requests != 1 {
			t.Fatalf("got %d requests, want 1", *requests)
		}

		makeStale(t, cachePath)
		outdatedVersionWarning(cfg, cachePath)
		if *requests != 2 {
			t.Fatalf("got %d requests after the cache went stale, want 2", *requests)
		}
	})

	t.Run("failed lookup is not retried until it is stale", func(t *testing.T) {
		setBuildTag(t, "6.4.1")
		cfg, requests := versionServer(t, http.StatusInternalServerError, "")
		cachePath := filepath.Join(t.TempDir(), "recommended-version")

		for range 2 {
			if warning := outdatedVersionWarning(cfg, cachePath); warning != "" {
				t.Fatalf("got warning %q, want none", warning)
			}
		}
		if *requests != 1 {
			t.Fatalf("got %d requests, want 1", *requests)
		}
	})

	t.Run("no lookup without a writable cache", func(t *testing.T) {
		setBuildTag(t, "6.4.1")
		cfg, requests := versionServer(t, http.StatusOK, "6.4.2")
		// A regular file where the cache directory should be.
		blocker := filepath.Join(t.TempDir(), "blocker")
		if err := os.WriteFile(blocker, nil, 0o600); err != nil {
			t.Fatal(err)
		}

		if warning := outdatedVersionWarning(cfg, filepath.Join(blocker, "recommended-version")); warning != "" {
			t.Fatalf("got warning %q, want none", warning)
		}
		if *requests != 0 {
			t.Fatalf("got %d requests, want 0", *requests)
		}
	})
}

func TestSkipVersionCheck(t *testing.T) {
	tests := []struct {
		name       string
		buildTag   string
		optOut     string
		subcommand string
		want       bool
	}{
		{name: "release build running a command", buildTag: "6.4.2", subcommand: "search", want: false},
		{name: "opted out", buildTag: "6.4.2", optOut: "1", subcommand: "search", want: true},
		{name: "dev build", buildTag: version.DefaultBuildTag, subcommand: "search", want: true},
		{name: "src version", buildTag: "6.4.2", subcommand: "version", want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setBuildTag(t, test.buildTag)
			t.Setenv("SRC_SKIP_VERSION_CHECK", test.optOut)

			if got := skipVersionCheck(test.subcommand); got != test.want {
				t.Errorf("skipVersionCheck(%q) = %v, want %v", test.subcommand, got, test.want)
			}
		})
	}
}

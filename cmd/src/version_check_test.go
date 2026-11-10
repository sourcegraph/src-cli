package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sourcegraph/src-cli/internal/api"
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
func versionServer(t *testing.T, status int, recommended string) (client api.Client, endpointURL *url.URL, requests *int) {
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
	endpointURL = mustParseURL(t, server.URL)
	return api.NewClient(api.ClientOpts{EndpointURL: endpointURL, AccessToken: "token", Out: io.Discard}), endpointURL, requests
}

// useTemporaryCacheDir points os.UserCacheDir at an empty directory.
func useTemporaryCacheDir(t *testing.T) {
	t.Helper()
	// Wherever os.UserCacheDir looks on this operating system.
	for _, name := range []string{"HOME", "XDG_CACHE_HOME", "LocalAppData"} {
		t.Setenv(name, t.TempDir())
	}
}

// captureStderr returns what run writes to os.Stderr.
func captureStderr(t *testing.T, run func()) string {
	t.Helper()
	stderr := os.Stderr
	defer func() { os.Stderr = stderr }()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = writer
	run()
	writer.Close()
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(output)
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
			client, endpointURL, _ := versionServer(t, test.status, test.recommended)

			warning := outdatedVersionWarning(recommendedVersion(client, filepath.Join(t.TempDir(), "recommended-version")), endpointURL)

			if !test.wantWarning {
				if warning != "" {
					t.Fatalf("got warning %q, want none", warning)
				}
				return
			}
			for _, want := range []string{test.current, test.recommended, endpointURL.String(), "SRC_SKIP_VERSION_CHECK"} {
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
		client, endpointURL, requests := versionServer(t, http.StatusOK, "6.4.2")
		cachePath := filepath.Join(t.TempDir(), "nested", "recommended-version")

		for range 2 {
			if warning := outdatedVersionWarning(recommendedVersion(client, cachePath), endpointURL); warning == "" {
				t.Fatal("got no warning, want one on every command")
			}
		}
		if *requests != 1 {
			t.Fatalf("got %d requests, want 1", *requests)
		}

		makeStale(t, cachePath)
		outdatedVersionWarning(recommendedVersion(client, cachePath), endpointURL)
		if *requests != 2 {
			t.Fatalf("got %d requests after the cache went stale, want 2", *requests)
		}
	})

	t.Run("upgrading silences the warning without another lookup", func(t *testing.T) {
		setBuildTag(t, "6.4.1")
		client, endpointURL, requests := versionServer(t, http.StatusOK, "6.4.2")
		cachePath := filepath.Join(t.TempDir(), "recommended-version")
		if warning := outdatedVersionWarning(recommendedVersion(client, cachePath), endpointURL); warning == "" {
			t.Fatal("got no warning before upgrading, want one")
		}

		setBuildTag(t, "6.4.2")
		if warning := outdatedVersionWarning(recommendedVersion(client, cachePath), endpointURL); warning != "" {
			t.Fatalf("got warning %q after upgrading, want none", warning)
		}
		if *requests != 1 {
			t.Fatalf("got %d requests, want 1", *requests)
		}
	})

	t.Run("failed lookup is not retried until it is stale", func(t *testing.T) {
		setBuildTag(t, "6.4.1")
		client, endpointURL, requests := versionServer(t, http.StatusInternalServerError, "")
		cachePath := filepath.Join(t.TempDir(), "recommended-version")

		for range 2 {
			if warning := outdatedVersionWarning(recommendedVersion(client, cachePath), endpointURL); warning != "" {
				t.Fatalf("got warning %q, want none", warning)
			}
		}
		if *requests != 1 {
			t.Fatalf("got %d requests, want 1", *requests)
		}
	})

	t.Run("no lookup without a writable cache", func(t *testing.T) {
		setBuildTag(t, "6.4.1")
		client, endpointURL, requests := versionServer(t, http.StatusOK, "6.4.2")
		// A regular file where the cache directory should be.
		blocker := filepath.Join(t.TempDir(), "blocker")
		if err := os.WriteFile(blocker, nil, 0o600); err != nil {
			t.Fatal(err)
		}

		if warning := outdatedVersionWarning(recommendedVersion(client, filepath.Join(blocker, "recommended-version")), endpointURL); warning != "" {
			t.Fatalf("got warning %q, want none", warning)
		}
		if *requests != 0 {
			t.Fatalf("got %d requests, want 0", *requests)
		}
	})
}

func TestSkipVersionCheck(t *testing.T) {
	tests := []struct {
		name     string
		buildTag string
		optOut   string
		want     bool
	}{
		{name: "release build", buildTag: "6.4.2", want: false},
		{name: "opted out", buildTag: "6.4.2", optOut: "1", want: true},
		{name: "dev build", buildTag: version.DefaultBuildTag, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setBuildTag(t, test.buildTag)
			t.Setenv("SRC_SKIP_VERSION_CHECK", test.optOut)

			if got := skipVersionCheck(); got != test.want {
				t.Errorf("skipVersionCheck() = %v, want %v", got, test.want)
			}
		})
	}
}

// TestVersionCommandChecksWithItsOwnLookup covers 'src version', which always
// asks the instance: it warns with that answer and caches it, instead of
// running the check that other commands get from config.apiClient.
func TestVersionCommandChecksWithItsOwnLookup(t *testing.T) {
	const warning = "Warning: src-cli 6.4.1 is older than 6.4.3"

	// setup returns a fake instance that recommends 6.4.3, and the path of a
	// cache that was told 6.4.2 a moment ago.
	setup := func(t *testing.T) (client api.Client, endpointURL *url.URL, requests *int, cachePath string) {
		t.Helper()
		setBuildTag(t, "6.4.1")
		t.Setenv("SRC_SKIP_VERSION_CHECK", "")
		useTemporaryCacheDir(t)
		client, endpointURL, requests = versionServer(t, http.StatusOK, "6.4.3")
		cachePath, err := recommendedVersionCachePath(endpointURL)
		if err != nil {
			t.Fatal(err)
		}
		if err := cacheRecommendedVersion(cachePath, "6.4.2"); err != nil {
			t.Fatal(err)
		}
		return client, endpointURL, requests, cachePath
	}

	t.Run("warns with the answer it gets, and caches it for later commands", func(t *testing.T) {
		client, endpointURL, requests, cachePath := setup(t)

		stderr := captureStderr(t, func() {
			// The check that config.apiClient runs for every command.
			maybeWarnOutdatedVersion(client, endpointURL, "version")
			if err := versionHandler(VersionArgs{Client: client, EndpointURL: endpointURL}); err != nil {
				t.Error(err)
			}
		})

		if got := strings.Count(stderr, "Warning:"); got != 1 || !strings.Contains(stderr, warning) {
			t.Errorf("got stderr %q, want only %q", stderr, warning)
		}
		if cached := recommendedVersion(client, cachePath); cached != "6.4.3" {
			t.Errorf("got cached version %q, want 6.4.3", cached)
		}
		if *requests != 1 {
			t.Errorf("got %d requests, want 1", *requests)
		}
	})

	t.Run("-client-only does not ask the instance", func(t *testing.T) {
		client, endpointURL, requests, cachePath := setup(t)

		stderr := captureStderr(t, func() {
			maybeWarnOutdatedVersion(client, endpointURL, "version")
			if err := versionHandler(VersionArgs{Client: client, EndpointURL: endpointURL, ClientOnly: true}); err != nil {
				t.Error(err)
			}
		})

		if stderr != "" {
			t.Errorf("got stderr %q, want none", stderr)
		}
		if cached := recommendedVersion(client, cachePath); cached != "6.4.2" {
			t.Errorf("got cached version %q, want 6.4.2 left alone", cached)
		}
		if *requests != 0 {
			t.Errorf("got %d requests, want 0", *requests)
		}
	})
}

// TestAPIClientWarnsAboutOutdatedVersion covers where the check is hooked in:
// config.apiClient, which every command that talks to the instance calls with
// its own flags.
func TestAPIClientWarnsAboutOutdatedVersion(t *testing.T) {
	tests := []struct {
		name               string
		insecureSkipVerify bool
		wantWarnings       int
	}{
		{name: "self-signed instance with -insecure-skip-verify is warned once, however many clients", insecureSkipVerify: true, wantWarnings: 1},
		{name: "self-signed instance without -insecure-skip-verify is not trusted", insecureSkipVerify: false, wantWarnings: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setBuildTag(t, "6.4.1")
			t.Setenv("SRC_SKIP_VERSION_CHECK", "")
			useTemporaryCacheDir(t)
			warnOutdatedVersionOnce = sync.Once{}
			t.Cleanup(func() { warnOutdatedVersionOnce = sync.Once{} })

			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, `{"version": "6.4.2"}`)
			}))
			server.Config.ErrorLog = log.New(io.Discard, "", 0) // rejected certificates
			server.StartTLS()
			t.Cleanup(server.Close)
			cfg := &config{endpointURL: mustParseURL(t, server.URL), accessToken: "token"}
			flags := api.NewFlagsFromValues(false, false, false, test.insecureSkipVerify, false)

			stderr := captureStderr(t, func() {
				for range 2 {
					cfg.apiClient(flags, io.Discard)
				}
			})

			if got := strings.Count(stderr, "Warning: src-cli 6.4.1 is older than 6.4.2"); got != test.wantWarnings {
				t.Errorf("got %d warnings, want %d; stderr: %q", got, test.wantWarnings, stderr)
			}
		})
	}
}

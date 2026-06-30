package main

import (
	"testing"

	"github.com/sourcegraph/src-cli/internal/version"
)

func TestIsOutdated(t *testing.T) {
	tests := []struct {
		name         string
		current      string
		recommended  string
		wantOutdated bool
		wantOK       bool
	}{
		{"older patch", "4.3.0", "4.3.1", true, true},
		{"older minor", "4.3.0", "4.4.0", true, true},
		{"older major", "3.43.0", "4.0.0", true, true},
		{"equal", "4.4.0", "4.4.0", false, true},
		{"newer", "4.5.0", "4.4.0", false, true},
		{"v-prefixed current", "v4.3.0", "4.4.0", true, true},
		{"prerelease, same base, not outdated", "4.4.0-rc.1", "4.4.0", false, true},
		{"prerelease, older base, outdated", "4.3.0-rc.1", "4.4.0", true, true},
		{"unparseable current", "dev", "4.4.0", false, false},
		{"empty recommended", "4.3.0", "", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOutdated, gotOK := isOutdated(tt.current, tt.recommended)
			if gotOutdated != tt.wantOutdated || gotOK != tt.wantOK {
				t.Errorf("isOutdated(%q, %q) = (%v, %v), want (%v, %v)",
					tt.current, tt.recommended, gotOutdated, gotOK, tt.wantOutdated, tt.wantOK)
			}
		})
	}
}

func TestShouldSkipVersionCheck(t *testing.T) {
	// pretend this is a real release build for the duration of the test, since the
	// default BuildTag ("dev") would otherwise short-circuit every case.
	orig := version.BuildTag
	version.BuildTag = "4.3.0"
	t.Cleanup(func() { version.BuildTag = orig })

	t.Run("skips for no-op subcommands", func(t *testing.T) {
		for _, sub := range []string{"", "help", "version", "-h", "--help"} {
			if !shouldSkipVersionCheck(sub) {
				t.Errorf("shouldSkipVersionCheck(%q) = false, want true", sub)
			}
		}
	})

	t.Run("runs for normal subcommands", func(t *testing.T) {
		for _, sub := range []string{"search", "batch", "api"} {
			if shouldSkipVersionCheck(sub) {
				t.Errorf("shouldSkipVersionCheck(%q) = true, want false", sub)
			}
		}
	})

	t.Run("skips when opt-out env var is set", func(t *testing.T) {
		t.Setenv(envSkipVersionCheck, "1")
		if !shouldSkipVersionCheck("search") {
			t.Errorf("shouldSkipVersionCheck with %s set = false, want true", envSkipVersionCheck)
		}
	})

	t.Run("skips for dev builds", func(t *testing.T) {
		version.BuildTag = version.DefaultBuildTag
		defer func() { version.BuildTag = "4.3.0" }()
		if !shouldSkipVersionCheck("search") {
			t.Error("shouldSkipVersionCheck for dev build = false, want true")
		}
	})
}

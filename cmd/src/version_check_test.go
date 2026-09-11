package main

import (
	"os"
	"testing"

	"github.com/sourcegraph/src-cli/internal/version"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name                string
		in                  string
		major, minor, patch int
		ok                  bool
	}{
		{"plain", "1.2.3", 1, 2, 3, true},
		{"leading v", "v1.2.3", 1, 2, 3, true},
		{"surrounding whitespace", "  1.2.3\n", 1, 2, 3, true},
		{"prerelease suffix ignored", "1.2.3-rc1", 1, 2, 3, true},
		{"build metadata ignored", "1.2.3+abc123", 1, 2, 3, true},
		{"prerelease and metadata ignored", "v1.2.3-rc1+abc", 1, 2, 3, true},
		{"multi-digit components", "10.20.30", 10, 20, 30, true},
		{"missing patch", "1.2", 0, 0, 0, false},
		{"extra component unparseable", "1.2.3.4", 0, 0, 0, false},
		{"empty", "", 0, 0, 0, false},
		{"non-numeric", "a.b.c", 0, 0, 0, false},
		{"partially numeric", "1.x.3", 0, 0, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			maj, min, patch, ok := parseVersion(tt.in)
			if ok != tt.ok {
				t.Fatalf("parseVersion(%q) ok = %v, want %v", tt.in, ok, tt.ok)
			}
			if !tt.ok {
				return // components are meaningless when ok is false
			}
			if maj != tt.major || min != tt.minor || patch != tt.patch {
				t.Errorf("parseVersion(%q) = (%d, %d, %d), want (%d, %d, %d)",
					tt.in, maj, min, patch, tt.major, tt.minor, tt.patch)
			}
		})
	}
}

func TestIsOlderVersion(t *testing.T) {
	tests := []struct {
		name        string
		current     string
		recommended string
		want        bool
	}{
		{"older patch", "1.2.3", "1.2.4", true},
		{"older minor", "1.2.3", "1.3.0", true},
		{"older major", "1.2.3", "2.0.0", true},
		{"equal", "1.2.3", "1.2.3", false},
		{"newer patch", "1.2.4", "1.2.3", false},
		{"newer major", "2.0.0", "1.9.9", false},
		{"prerelease on current, same core", "1.2.3-rc1", "1.2.3", false},
		{"prerelease on recommended, same core", "1.2.3", "1.2.3-rc1", false},
		{"leading v tolerated", "v1.2.3", "1.2.4", true},
		{"unparseable current fails open", "garbage", "1.2.3", false},
		{"unparseable recommended fails open", "1.2.3", "garbage", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isOlderVersion(tt.current, tt.recommended); got != tt.want {
				t.Errorf("isOlderVersion(%q, %q) = %v, want %v",
					tt.current, tt.recommended, got, tt.want)
			}
		})
	}
}

func TestSkipVersionCheck(t *testing.T) {
	// Ensure the opt-out env var is absent for the non-env cases, restoring
	// whatever the developer's environment had afterward.
	if orig, ok := os.LookupEnv("SRC_SKIP_VERSION_CHECK"); ok {
		os.Unsetenv("SRC_SKIP_VERSION_CHECK")
		t.Cleanup(func() { os.Setenv("SRC_SKIP_VERSION_CHECK", orig) })
	}

	// Pretend this is a release build for everything except the dev-build case.
	origTag := version.BuildTag
	t.Cleanup(func() { version.BuildTag = origTag })
	version.BuildTag = "5.1.2"

	t.Run("dev build is skipped", func(t *testing.T) {
		version.BuildTag = version.DefaultBuildTag
		defer func() { version.BuildTag = "5.1.2" }()

		if !skipVersionCheck([]string{"search", "foo"}) {
			t.Error("expected dev build to be skipped")
		}
	})

	t.Run("opt-out env var is skipped", func(t *testing.T) {
		t.Setenv("SRC_SKIP_VERSION_CHECK", "1")

		if !skipVersionCheck([]string{"search", "foo"}) {
			t.Error("expected SRC_SKIP_VERSION_CHECK to skip the check")
		}
	})

	t.Run("empty env value still counts as set", func(t *testing.T) {
		t.Setenv("SRC_SKIP_VERSION_CHECK", "")

		if !skipVersionCheck([]string{"search", "foo"}) {
			t.Error("expected a present-but-empty env var to skip the check")
		}
	})

	argCases := []struct {
		name string
		args []string
		want bool
	}{
		{"no args", nil, true},
		{"version subcommand", []string{"version"}, true},
		{"help subcommand", []string{"help"}, true},
		{"-h flag", []string{"-h"}, true},
		{"-help flag", []string{"-help"}, true},
		{"--help flag", []string{"--help"}, true},
		{"version after global flag", []string{"-v", "version"}, true},
		{"normal command", []string{"search", "-json", "foo"}, false},
		{"normal multi-arg command", []string{"batch", "preview", "-f", "x.yaml"}, false},
	}

	for _, tt := range argCases {
		t.Run(tt.name, func(t *testing.T) {
			if got := skipVersionCheck(tt.args); got != tt.want {
				t.Errorf("skipVersionCheck(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

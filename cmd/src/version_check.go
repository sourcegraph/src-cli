package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/version"
)

// versionCheckTimeout bounds the best-effort recommended-version lookup so it
// never noticeably delays a command.
const versionCheckTimeout = 3 * time.Second

// maybeWarnVersion performs a best-effort check of the running src-cli version
// against the version recommended by the configured Sourcegraph instance and
// prints a single warning to stderr if src-cli is behind.
//
// It is entirely fail-open: any error (missing config, network failure,
// unreachable instance, unparseable version) results in no output. It writes
// only to stderr, so --json output on stdout is unaffected.
func maybeWarnVersion(args []string) {
	if skipVersionCheck(args) {
		return
	}

	// Read config independently of command dispatch. Note: this runs before
	// flag parsing, so -endpoint/-config flags are not honored here; the
	// endpoint is resolved from SRC_ENDPOINT, the config file, or the default.
	cfg, err := readConfig()
	if err != nil {
		return // fail-open: missing or invalid config
	}

	ctx, cancel := context.WithTimeout(context.Background(), versionCheckTimeout)
	defer cancel()

	client := cfg.apiClient(&api.Flags{}, os.Stderr)
	recommended, err := getRecommendedVersion(ctx, client)
	if err != nil || recommended == "" {
		return // fail-open: network error, unreachable, or unsupported instance
	}

	if isOlderVersion(version.BuildTag, recommended) {
		fmt.Fprintf(os.Stderr,
			"warning: src-cli v%s is older than the version recommended by your Sourcegraph instance (v%s). "+
				"Run `src version` for details, or set SRC_SKIP_VERSION_CHECK=1 to silence this.\n",
			version.BuildTag, recommended)
	}
}

// skipVersionCheck reports whether the version check should be skipped for this
// invocation. Dev builds, an explicit opt-out env var, and
// version/help/no-arg invocations are all skipped.
func skipVersionCheck(args []string) bool {
	// Dev builds have no meaningful version to compare.
	if version.BuildTag == version.DefaultBuildTag {
		return true
	}

	// Explicit opt-out.
	if _, ok := os.LookupEnv("SRC_SKIP_VERSION_CHECK"); ok {
		return true
	}

	// No subcommand: `src` alone just prints usage.
	if len(args) == 0 {
		return true
	}

	// version and help invocations already surface version info / need no nag.
	for _, arg := range args {
		switch arg {
		case "version", "help", "-h", "-help", "--help":
			return true
		}
	}

	return false
}

// isOlderVersion reports whether current is strictly older than recommended,
// comparing only major/minor/patch so prereleases do not trigger spurious
// warnings. If either version is unparseable it returns false (fail-open).
func isOlderVersion(current, recommended string) bool {
	cMaj, cMin, cPatch, ok := parseVersion(current)
	if !ok {
		return false
	}
	rMaj, rMin, rPatch, ok := parseVersion(recommended)
	if !ok {
		return false
	}

	if cMaj != rMaj {
		return cMaj < rMaj
	}
	if cMin != rMin {
		return cMin < rMin
	}
	return cPatch < rPatch
}

// parseVersion extracts the major, minor, and patch components from a semver
// string, tolerating a leading "v" and ignoring any prerelease or build
// metadata suffix (e.g. "v1.2.3-rc1+meta" -> 1, 2, 3).
func parseVersion(s string) (major, minor, patch int, ok bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")

	// Drop prerelease and build metadata: 1.2.3-rc1+meta -> 1.2.3
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}

	parts := strings.SplitN(s, ".", 3)
	if len(parts) != 3 {
		return 0, 0, 0, false
	}

	var err error
	if major, err = strconv.Atoi(parts[0]); err != nil {
		return 0, 0, 0, false
	}
	if minor, err = strconv.Atoi(parts[1]); err != nil {
		return 0, 0, 0, false
	}
	if patch, err = strconv.Atoi(parts[2]); err != nil {
		return 0, 0, 0, false
	}
	return major, minor, patch, true
}

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Masterminds/semver"

	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/version"
)

// envSkipVersionCheck disables the automatic out-of-date warning when set to a
// non-empty value.
const envSkipVersionCheck = "SRC_SKIP_VERSION_CHECK"

// versionCheckTimeout bounds how long the background version check may take so it
// never noticeably delays the command the user actually asked to run.
const versionCheckTimeout = 3 * time.Second

// maybeWarnOutdatedVersion checks, on a best-effort basis, whether the running
// src-cli is older than the version recommended by the configured Sourcegraph
// instance and prints a warning to stderr if so.
//
// It is intentionally fail-open: any problem (missing config, network failure,
// unparseable version, ...) results in no warning, and it never blocks or delays
// the actual command by more than versionCheckTimeout.
func maybeWarnOutdatedVersion(subcommand string) {
	if shouldSkipVersionCheck(subcommand) {
		return
	}

	// Resolve config independently and ignore any error: a malformed config is the
	// dispatched command's problem to report, not ours.
	c, err := readConfig()
	if err != nil || c == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), versionCheckTimeout)
	defer cancel()

	// Build a quiet client: discard incidental output and skip user-agent telemetry
	// for this background request.
	client := c.apiClient(api.NewFlagsFromValues(false, false, false, false, false), io.Discard)

	recommended, err := getRecommendedVersion(ctx, client)
	if err != nil || recommended == "" {
		return
	}

	if outdated, ok := isOutdated(version.BuildTag, recommended); ok && outdated {
		fmt.Fprintf(os.Stderr,
			"Warning: src-cli is out of date: you are running %s, but your Sourcegraph "+
				"instance recommends %s or later. Some commands may not work as expected; see "+
				"https://github.com/sourcegraph/src-cli#installation to upgrade. "+
				"Set %s=1 to silence this warning.\n",
			version.BuildTag, recommended, envSkipVersionCheck)
	}
}

// shouldSkipVersionCheck reports whether the version check should be skipped for
// this invocation.
func shouldSkipVersionCheck(subcommand string) bool {
	// Respect an explicit opt-out (also the simplest way to silence the warning in
	// CI or scripts).
	if os.Getenv(envSkipVersionCheck) != "" {
		return true
	}

	// Dev builds have no meaningful version to compare against.
	if version.BuildTag == version.DefaultBuildTag {
		return true
	}

	// Nothing to do for help/version/no-subcommand invocations. `version` already
	// prints the recommended version itself.
	switch subcommand {
	case "", "help", "version":
		return true
	}

	// A leading dash means no subcommand was given (e.g. `src -h`).
	if strings.HasPrefix(subcommand, "-") {
		return true
	}

	return false
}

// isOutdated reports whether current is an older release than recommended,
// comparing only the major/minor/patch components so that prerelease or build
// metadata never produces a spurious warning.
//
// ok is false when either version cannot be parsed, in which case callers should
// not warn.
func isOutdated(current, recommended string) (outdated bool, ok bool) {
	cur, err := semver.NewVersion(current)
	if err != nil {
		return false, false
	}
	rec, err := semver.NewVersion(recommended)
	if err != nil {
		return false, false
	}

	curParts := [3]int64{cur.Major(), cur.Minor(), cur.Patch()}
	recParts := [3]int64{rec.Major(), rec.Minor(), rec.Patch()}
	for i := range curParts {
		if curParts[i] != recParts[i] {
			return curParts[i] < recParts[i], true
		}
	}
	return false, true
}

package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Masterminds/semver"

	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/version"
)

const (
	// versionCheckTimeout bounds the recommended-version lookup, which runs
	// before the command the user asked for.
	versionCheckTimeout = 3 * time.Second

	// versionCheckInterval is how long the result of a lookup is reused before
	// the instance is asked again.
	versionCheckInterval = 24 * time.Hour
)

// warnOutdatedVersionOnce limits the check to once per run of src-cli, however
// many API clients the command creates.
var warnOutdatedVersionOnce sync.Once

// maybeWarnOutdatedVersion prints a warning to stderr when src-cli is older
// than the version recommended by the Sourcegraph instance at endpointURL,
// which it asks using client.
//
// It never fails the command: an unreachable instance, an unparseable version,
// or an unwritable cache all result in no warning.
func maybeWarnOutdatedVersion(client api.Client, endpointURL *url.URL, subcommand string) {
	if skipVersionCheck(subcommand) {
		return
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return
	}
	cachePath := filepath.Join(cacheDir, "sourcegraph", "src-cli", "recommended-version-"+url.QueryEscape(endpointURL.String()))
	if warning := outdatedVersionWarning(client, endpointURL, cachePath); warning != "" {
		fmt.Fprintln(os.Stderr, warning)
	}
}

// skipVersionCheck reports whether the version check is disabled, has no
// version to compare, or is redundant because the command is 'src version'.
func skipVersionCheck(subcommand string) bool {
	return os.Getenv("SRC_SKIP_VERSION_CHECK") != "" ||
		version.BuildTag == version.DefaultBuildTag ||
		subcommand == "version"
}

// outdatedVersionWarning returns the warning to print, or "" when src-cli is
// not known to be older than the version the instance recommends.
func outdatedVersionWarning(client api.Client, endpointURL *url.URL, cachePath string) string {
	current, err := semver.NewVersion(version.BuildTag)
	if err != nil {
		return ""
	}
	recommended, err := semver.NewVersion(recommendedVersion(client, cachePath))
	if err != nil {
		return ""
	}
	if !current.LessThan(recommended) {
		return ""
	}
	return fmt.Sprintf("⚠️  Warning: src-cli %s is older than %s, the version recommended by %s. "+
		"To upgrade, see https://github.com/sourcegraph/src-cli#installation. "+
		"To silence this warning, set SRC_SKIP_VERSION_CHECK=1.",
		current, recommended, endpointURL)
}

// recommendedVersion returns the src-cli version recommended by the instance
// that client talks to, or "" when it is unknown. The answer is cached in the
// file at cachePath for versionCheckInterval, so most commands make no extra
// request.
func recommendedVersion(client api.Client, cachePath string) string {
	if info, err := os.Stat(cachePath); err == nil && time.Since(info.ModTime()) < versionCheckInterval {
		cached, _ := os.ReadFile(cachePath)
		return string(cached)
	}

	// Record the attempt before the lookup, so that an instance that does not
	// answer delays one command per versionCheckInterval instead of every
	// command. Without a writable cache there is no such bound, so do not look.
	if os.MkdirAll(filepath.Dir(cachePath), 0o700) != nil || os.WriteFile(cachePath, nil, 0o600) != nil {
		return ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), versionCheckTimeout)
	defer cancel()
	recommended, err := getRecommendedVersion(ctx, client)
	if err != nil {
		return ""
	}
	_ = os.WriteFile(cachePath, []byte(recommended), 0o600)
	return recommended
}

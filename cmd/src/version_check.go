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
// which it asks using client unless a recent answer is cached.
//
// It never fails the command: an unreachable instance, an unparseable version,
// or an unwritable cache all result in no warning.
func maybeWarnOutdatedVersion(client api.Client, endpointURL *url.URL, subcommand string) {
	// 'src version' asks the instance itself, and then calls
	// warnOutdatedVersionFromLookup with the answer.
	if subcommand == "version" || skipVersionCheck() {
		return
	}
	cachePath, err := recommendedVersionCachePath(endpointURL)
	if err != nil {
		return
	}
	printOutdatedVersionWarning(recommendedVersion(client, cachePath), endpointURL)
}

// warnOutdatedVersionFromLookup is the check for 'src version', which has just
// been told the recommended version by the instance at endpointURL. The answer
// is cached, so that the commands that follow warn about the same version.
func warnOutdatedVersionFromLookup(recommended string, endpointURL *url.URL) {
	if skipVersionCheck() {
		return
	}
	if cachePath, err := recommendedVersionCachePath(endpointURL); err == nil {
		_ = cacheRecommendedVersion(cachePath, recommended)
	}
	printOutdatedVersionWarning(recommended, endpointURL)
}

// skipVersionCheck reports whether the version check is disabled, or has no
// version to compare.
func skipVersionCheck() bool {
	return os.Getenv("SRC_SKIP_VERSION_CHECK") != "" || version.BuildTag == version.DefaultBuildTag
}

func printOutdatedVersionWarning(recommended string, endpointURL *url.URL) {
	if warning := outdatedVersionWarning(recommended, endpointURL); warning != "" {
		fmt.Fprintln(os.Stderr, warning)
	}
}

// outdatedVersionWarning returns the warning to print, or "" when src-cli is
// not known to be older than the recommended version.
func outdatedVersionWarning(recommendedVersion string, endpointURL *url.URL) string {
	current, err := semver.NewVersion(version.BuildTag)
	if err != nil {
		return ""
	}
	recommended, err := semver.NewVersion(recommendedVersion)
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

// recommendedVersionCachePath returns the file that holds the version
// recommended by the instance at endpointURL.
func recommendedVersionCachePath(endpointURL *url.URL) (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cacheDir, "sourcegraph", "src-cli", "recommended-version-"+url.QueryEscape(endpointURL.String())), nil
}

// cacheRecommendedVersion records recommended, and the time it was learned, in
// the file at cachePath.
func cacheRecommendedVersion(cachePath, recommended string) error {
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o700); err != nil {
		return err
	}
	return os.WriteFile(cachePath, []byte(recommended), 0o600)
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
	if cacheRecommendedVersion(cachePath, "") != nil {
		return ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), versionCheckTimeout)
	defer cancel()
	recommended, err := getRecommendedVersion(ctx, client)
	if err != nil {
		return ""
	}
	_ = cacheRecommendedVersion(cachePath, recommended)
	return recommended
}

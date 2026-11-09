package main

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/Masterminds/semver"

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

// maybeWarnOutdatedVersion prints a warning to stderr when src-cli is older
// than the version recommended by the configured Sourcegraph instance.
//
// It never fails the command: an unreachable instance, an unparseable version,
// or an unwritable cache all result in no warning.
func maybeWarnOutdatedVersion(cfg *config, subcommand string) {
	if skipVersionCheck(subcommand) {
		return
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return
	}
	cachePath := filepath.Join(cacheDir, "sourcegraph", "src-cli", "recommended-version-"+url.QueryEscape(cfg.endpointURL.String()))
	if warning := outdatedVersionWarning(cfg, cachePath); warning != "" {
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
func outdatedVersionWarning(cfg *config, cachePath string) string {
	current, err := semver.NewVersion(version.BuildTag)
	if err != nil {
		return ""
	}
	recommended, err := semver.NewVersion(recommendedVersion(cfg, cachePath))
	if err != nil {
		return ""
	}
	if !current.LessThan(recommended) {
		return ""
	}
	return fmt.Sprintf("⚠️  Warning: src-cli %s is older than %s, the version recommended by %s. "+
		"To upgrade, see https://github.com/sourcegraph/src-cli#installation. "+
		"To silence this warning, set SRC_SKIP_VERSION_CHECK=1.",
		current, recommended, cfg.endpointURL)
}

// recommendedVersion returns the src-cli version recommended by the configured
// instance, or "" when it is unknown. The answer is cached in the file at
// cachePath for versionCheckInterval, so most commands make no extra request.
func recommendedVersion(cfg *config, cachePath string) string {
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
	recommended, err := getRecommendedVersion(ctx, cfg.apiClient(nil, io.Discard))
	if err != nil {
		return ""
	}
	_ = os.WriteFile(cachePath, []byte(recommended), 0o600)
	return recommended
}

package blueprint

import (
	"context"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/sourcegraph/sourcegraph/lib/errors"
)

// BlueprintSource provides access to blueprint files from various sources.
type BlueprintSource interface {
	// Prepare makes the blueprint directory available and returns its path.
	// The cleanup function MUST be called when done (may be nil for sources
	// that don't require cleanup).
	Prepare(ctx context.Context) (blueprintDir string, cleanup func() error, err error)
}

// ResolveBlueprintSource returns a BlueprintSource for the given repository or path and subdirectory.
// rawRepo may be an HTTPS Git URL or a local filesystem path. rev may be empty to use the default branch.
func ResolveBlueprintSource(rawRepo, rev, subdir string) (BlueprintSource, error) {
	if err := validateSubdir(subdir); err != nil {
		return nil, err
	}

	if isLocalPath(rawRepo) {
		path := rawRepo
		if !filepath.IsAbs(path) {
			absPath, err := filepath.Abs(path)
			if err != nil {
				return nil, errors.Wrapf(err, "resolving absolute path for %q", path)
			}
			path = absPath
		}
		return &LocalBlueprintSource{
			Path:   path,
			Subdir: subdir,
		}, nil
	}

	u, err := url.Parse(rawRepo)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, errors.Newf("invalid repository URL %q: must be an HTTPS Git URL or local path", rawRepo)
	}

	if u.Scheme != "https" {
		return nil, errors.Newf("unsupported URL scheme %q: only HTTPS is allowed", u.Scheme)
	}

	return &GitBlueprintSource{
		RepoURL: rawRepo,
		Rev:     rev,
		Subdir:  subdir,
	}, nil
}

func isLocalPath(s string) bool {
	if filepath.IsAbs(s) {
		return true
	}
	if strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../") || s == "." || s == ".." {
		return true
	}
	return false
}

// ResolveRootSource returns a BlueprintSource that provides the repository root
// without requiring a blueprint.yaml file. Used for listing all blueprints in a repository.
func ResolveRootSource(rawRepo, rev string) (BlueprintSource, error) {
	if isLocalPath(rawRepo) {
		path := rawRepo
		if !filepath.IsAbs(path) {
			absPath, err := filepath.Abs(path)
			if err != nil {
				return nil, errors.Wrapf(err, "resolving absolute path for %q", path)
			}
			path = absPath
		}
		return &LocalRootSource{Path: path}, nil
	}

	u, err := url.Parse(rawRepo)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, errors.Newf("invalid repository URL %q: must be an HTTPS Git URL or local path", rawRepo)
	}

	if u.Scheme != "https" {
		return nil, errors.Newf("unsupported URL scheme %q: only HTTPS is allowed", u.Scheme)
	}

	return &GitRootSource{
		RepoURL: rawRepo,
		Rev:     rev,
	}, nil
}

func validateSubdir(subdir string) error {
	if subdir == "" {
		return nil
	}

	cleaned := filepath.Clean(subdir)
	if filepath.IsAbs(cleaned) {
		return errors.Newf("subdir must be a relative path, got %q", subdir)
	}
	if strings.HasPrefix(cleaned, "..") {
		return errors.Newf("subdir must not escape the repository root, got %q", subdir)
	}

	return nil
}

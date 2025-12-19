package blueprint

import (
	"context"
	"os"
	"path/filepath"

	"github.com/sourcegraph/sourcegraph/lib/errors"
)

// LocalBlueprintSource provides access to blueprint files from a local directory.
type LocalBlueprintSource struct {
	Path   string
	Subdir string
}

func (s *LocalBlueprintSource) Prepare(ctx context.Context) (string, func() error, error) {
	blueprintDir := s.Path
	if s.Subdir != "" {
		blueprintDir = filepath.Join(s.Path, filepath.Clean(s.Subdir))
	}

	info, err := os.Stat(blueprintDir)
	if err != nil {
		return "", nil, errors.Wrapf(err, "blueprint directory %q not accessible", blueprintDir)
	}
	if !info.IsDir() {
		return "", nil, errors.Newf("blueprint path %q is not a directory", blueprintDir)
	}

	if _, err := os.Stat(filepath.Join(blueprintDir, "blueprint.yaml")); err != nil {
		return "", nil, errors.Wrap(err, "blueprint.yaml not found in directory")
	}

	return blueprintDir, nil, nil
}

// LocalRootSource provides access to a local directory without requiring
// a blueprint.yaml file at the root. Used for listing all blueprints.
type LocalRootSource struct {
	Path string
}

func (s *LocalRootSource) Prepare(ctx context.Context) (string, func() error, error) {
	info, err := os.Stat(s.Path)
	if err != nil {
		return "", nil, errors.Wrapf(err, "directory %q not accessible", s.Path)
	}
	if !info.IsDir() {
		return "", nil, errors.Newf("path %q is not a directory", s.Path)
	}

	return s.Path, nil, nil
}

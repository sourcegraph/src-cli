package blueprint

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sourcegraph/sourcegraph/lib/errors"
)

// GitBlueprintSource clones a Git repository over HTTPS and provides access
// to the blueprint files within it.
type GitBlueprintSource struct {
	RepoURL   string
	Rev       string
	Subdir    string
	GitBinary string // defaults to "git" if empty
}

func (s *GitBlueprintSource) Prepare(ctx context.Context) (string, func() error, error) {
	git := s.GitBinary
	if git == "" {
		git = "git"
	}

	if _, err := exec.LookPath(git); err != nil {
		return "", nil, errors.New("git CLI not found; please install git to use 'src blueprint import'")
	}

	tmpDir, err := os.MkdirTemp("", "src-blueprint-*")
	if err != nil {
		return "", nil, errors.Wrap(err, "creating temporary directory for blueprint clone")
	}

	cleanup := func() error {
		return os.RemoveAll(tmpDir)
	}

	if err := s.clone(ctx, git, tmpDir); err != nil {
		_ = cleanup()
		return "", nil, err
	}

	if s.Rev != "" {
		if err := s.checkout(ctx, git, tmpDir); err != nil {
			_ = cleanup()
			return "", nil, err
		}
	}

	blueprintDir := tmpDir
	if s.Subdir != "" {
		blueprintDir = filepath.Join(tmpDir, filepath.Clean(s.Subdir))
	}

	if _, err := os.Stat(filepath.Join(blueprintDir, "blueprint.yaml")); err != nil {
		_ = cleanup()
		return "", nil, errors.Wrap(err, "blueprint.yaml not found in cloned repository")
	}

	return blueprintDir, cleanup, nil
}

func (s *GitBlueprintSource) clone(ctx context.Context, git, targetDir string) error {
	args := []string{"clone"}
	if s.Rev == "" {
		args = append(args, "--branch", "main")
	}
	args = append(args, "--", s.RepoURL, targetDir)
	cmd := exec.CommandContext(ctx, git, args...)
	cmd.Env = gitEnv()
	cmd.Stdout = io.Discard

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return errors.Wrapf(err, "git clone %q failed: %s", s.RepoURL, strings.TrimSpace(stderr.String()))
	}

	return nil
}

func (s *GitBlueprintSource) checkout(ctx context.Context, git, repoDir string) error {
	args := []string{"-C", repoDir, "checkout", "--detach", s.Rev}
	cmd := exec.CommandContext(ctx, git, args...)
	cmd.Env = gitEnv()
	cmd.Stdout = io.Discard

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return errors.Wrapf(err, "git checkout %q failed: %s", s.Rev, strings.TrimSpace(stderr.String()))
	}

	return nil
}

func gitEnv() []string {
	env := os.Environ()
	env = append(env, "GIT_TERMINAL_PROMPT=0")
	return env
}

// GitRootSource clones a Git repository and provides access to the root
// without requiring a blueprint.yaml file. Used for listing all blueprints.
type GitRootSource struct {
	RepoURL   string
	Rev       string
	GitBinary string
}

func (s *GitRootSource) Prepare(ctx context.Context) (string, func() error, error) {
	git := s.GitBinary
	if git == "" {
		git = "git"
	}

	if _, err := exec.LookPath(git); err != nil {
		return "", nil, errors.New("git CLI not found; please install git to use 'src blueprint list'")
	}

	tmpDir, err := os.MkdirTemp("", "src-blueprint-*")
	if err != nil {
		return "", nil, errors.Wrap(err, "creating temporary directory for blueprint clone")
	}

	cleanup := func() error {
		return os.RemoveAll(tmpDir)
	}

	src := &GitBlueprintSource{
		RepoURL:   s.RepoURL,
		Rev:       s.Rev,
		GitBinary: git,
	}

	if err := src.clone(ctx, git, tmpDir); err != nil {
		_ = cleanup()
		return "", nil, err
	}

	if s.Rev != "" {
		if err := src.checkout(ctx, git, tmpDir); err != nil {
			_ = cleanup()
			return "", nil, err
		}
	}

	return tmpDir, cleanup, nil
}

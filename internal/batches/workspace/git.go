package workspace

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sourcegraph/sourcegraph/lib/errors"
)

type gitMetadataSnapshot struct {
	dotGit         *gitControlFile
	config         *gitControlFile
	configWorktree *gitControlFile
}

type gitControlFile struct {
	contents []byte
	mode     os.FileMode
}

func snapshotGitMetadata(dir string) (*gitMetadataSnapshot, error) {
	dotGit := filepath.Join(dir, ".git")
	info, err := os.Lstat(dotGit)
	if err != nil {
		return nil, err
	}

	snapshot := &gitMetadataSnapshot{}
	switch {
	case info.Mode().IsRegular():
		snapshot.dotGit, err = snapshotGitControlFile(dotGit)
	case info.IsDir():
		snapshot.config, err = snapshotGitControlFile(filepath.Join(dotGit, "config"))
		if err == nil {
			snapshot.configWorktree, err = snapshotOptionalGitControlFile(filepath.Join(dotGit, "config.worktree"))
		}
	default:
		return nil, fmt.Errorf("%s is not a regular file or directory", dotGit)
	}
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

func snapshotOptionalGitControlFile(path string) (*gitControlFile, error) {
	file, err := snapshotGitControlFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	return file, err
}

func snapshotGitControlFile(path string) (*gitControlFile, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return &gitControlFile{contents: contents, mode: info.Mode()}, nil
}

func (s *gitMetadataSnapshot) restore(dir string) error {
	if s == nil {
		return nil
	}

	dotGit := filepath.Join(dir, ".git")
	if s.dotGit != nil {
		return restoreGitControlFile(dotGit, s.dotGit)
	}

	info, err := os.Lstat(dotGit)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s is no longer a directory", dotGit)
	}
	if err := restoreGitControlFile(filepath.Join(dotGit, "config"), s.config); err != nil {
		return err
	}
	return restoreGitControlFile(filepath.Join(dotGit, "config.worktree"), s.configWorktree)
}

func restoreGitControlFile(path string, snapshot *gitControlFile) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	if snapshot == nil {
		return nil
	}

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, snapshot.mode.Perm())
	if err != nil {
		return err
	}
	if _, err := file.Write(snapshot.contents); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

func runGitCmd(ctx context.Context, dir string, args ...string) ([]byte, error) {
	// Repository contents are untrusted. Keep hooks disabled even if a command
	// encounters an attacker-controlled local Git configuration.
	args = append([]string{"-c", "core.hooksPath=/dev/null"}, args...)
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = []string{
		// Don't use the system wide git config.
		"GIT_CONFIG_NOSYSTEM=1",
		// And also not any other, because they can mess up output, change defaults, .. which can do unexpected things.
		"GIT_CONFIG=/dev/null",
		// Don't ask interactively for credentials.
		"GIT_TERMINAL_PROMPT=0",
		// Set user.name and user.email in the local repository. The user name and
		// e-mail will eventually be ignored anyway, since we're just using the Git
		// repository to generate diffs, but we don't want git to generate alarming
		// looking warnings.
		"GIT_AUTHOR_NAME=Sourcegraph",
		"GIT_AUTHOR_EMAIL=batch-changes@sourcegraph.com",
		"GIT_COMMITTER_NAME=Sourcegraph",
		"GIT_COMMITTER_EMAIL=batch-changes@sourcegraph.com",
	}
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return out, errors.Wrapf(err, "'git %s' failed: %s", strings.Join(args, " "), string(exitErr.Stderr))
		}
		return out, errors.Wrapf(err, "'git %s' failed: %s", strings.Join(args, " "), string(out))
	}
	return out, nil
}

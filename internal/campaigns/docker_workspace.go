package campaigns

import (
	"bytes"
	"context"
	"io/ioutil"
	"os"
	"os/exec"

	"github.com/pkg/errors"
	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/campaigns/graphql"
)

// dockerVolumeWorkspaceCreator creates dockerWorkspace instances.
type dockerVolumeWorkspaceCreator struct {
	client api.Client
}

var _ WorkspaceCreator = &dockerVolumeWorkspaceCreator{}

func (wc *dockerVolumeWorkspaceCreator) Create(ctx context.Context, repo *graphql.Repository) (Workspace, error) {
	w := &dockerVolumeWorkspace{}
	return w, w.init(ctx, wc.client, repo)
}

// dockerVolumeWorkspace workspaces are placed on Docker volumes (surprise!),
// and are therefore transparent to the host filesystem. This has performance
// advantages if bind mounts are slow, such as on Docker for Mac, but could make
// debugging harder and is slower when it's time to actually retrieve the diff.
type dockerVolumeWorkspace struct {
	volume string
}

var _ Workspace = &dockerVolumeWorkspace{}

func (w *dockerVolumeWorkspace) init(ctx context.Context, client api.Client, repo *graphql.Repository) error {
	// Download the ZIP archive to a temporary file.
	f, err := ioutil.TempFile(os.TempDir(), "src-archive-*.zip")
	if err != nil {
		return errors.Wrap(err, "creating temporary archive")
	}
	hostZip := f.Name()
	defer os.Remove(hostZip)

	if err := fetchRepositoryArchive(ctx, client, repo, hostZip); err != nil {
		return errors.Wrap(err, "fetching repository archive")
	}

	// Create a Docker volume.
	out, err := exec.CommandContext(ctx, "docker", "volume", "create").CombinedOutput()
	if err != nil {
		return errors.Wrap(err, "creating Docker volume")
	}
	w.volume = string(bytes.TrimSpace(out))

	// Now mount that temporary file into a Docker container, and unzip it into
	// the volume.
	common, err := w.DockerRunOpts(ctx, "/work")
	if err != nil {
		return errors.Wrap(err, "generating run options")
	}

	opts := append([]string{
		"run",
		"--rm",
		"--init",
		"--workdir", "/work",
		"--mount", "type=bind,source=" + hostZip + ",target=/tmp/zip,ro",
	}, common...)
	opts = append(opts, dockerWorkspaceImage, "unzip", "/tmp/zip")

	out, err = exec.CommandContext(ctx, "docker", opts...).CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "unzip output:\n\n%s\n\n", string(out))
	}

	return nil
}

func (w *dockerVolumeWorkspace) Close(ctx context.Context) error {
	// Cleanup here is easy: we just get rid of the Docker volume.
	return exec.CommandContext(ctx, "docker", "volume", "rm", w.volume).Run()
}

func (w *dockerVolumeWorkspace) DockerRunOpts(ctx context.Context, target string) ([]string, error) {
	return []string{
		"--mount", "type=volume,source=" + w.volume + ",target=" + target,
	}, nil
}

func (w *dockerVolumeWorkspace) Prepare(ctx context.Context) error {
	script := `#!/bin/sh
	
set -e
set -x

git init
# --force because we want previously "gitignored" files in the repository
git add --force --all
git commit --quiet --all -m src-action-exec
`

	if _, err := w.runScript(ctx, "/work", script); err != nil {
		return errors.Wrap(err, "preparing workspace")
	}
	return nil
}

func (w *dockerVolumeWorkspace) Changes(ctx context.Context) (*StepChanges, error) {
	script := `#!/bin/sh

set -e
# No set -x here, since we're going to parse the git status output.

git add --all > /dev/null
exec git status --porcelain
`

	out, err := w.runScript(ctx, "/work", script)
	if err != nil {
		return nil, errors.Wrap(err, "running git status")
	}

	changes, err := parseGitStatus(out)
	if err != nil {
		return nil, errors.Wrapf(err, "parsing git status output:\n\n%s", string(out))
	}

	return &changes, nil
}

func (w *dockerVolumeWorkspace) Diff(ctx context.Context) ([]byte, error) {
	// As of Sourcegraph 3.14 we only support unified diff format.
	// That means we need to strip away the `a/` and `/b` prefixes with `--no-prefix`.
	// See: https://github.com/sourcegraph/sourcegraph/blob/82d5e7e1562fef6be5c0b17f18631040fd330835/enterprise/internal/campaigns/service.go#L324-L329
	//
	// Also, we need to add --binary so binary file changes are inlined in the patch.
	script := `#!/bin/sh
	
exec git diff --cached --no-prefix --binary
`

	out, err := w.runScript(ctx, "/work", script)
	if err != nil {
		return nil, errors.Wrapf(err, "git diff:\n\n%s", string(out))
	}

	return out, nil
}

// dockerWorkspaceImage is the Docker image we'll run our unzip and git commands
// in. This needs to match the name defined in .github/workflows/docker.yml.
const dockerWorkspaceImage = "sourcegraph/src-campaign-volume-workspace"

// runScript is a utility function to mount the given shell script into a Docker
// container started from the dockerWorkspaceImage, then run it and return the
// output.
func (w *dockerVolumeWorkspace) runScript(ctx context.Context, target, script string) ([]byte, error) {
	f, err := ioutil.TempFile(os.TempDir(), "src-run-*")
	if err != nil {
		return nil, errors.Wrap(err, "creating run script")
	}
	name := f.Name()
	defer os.Remove(name)

	if _, err := f.WriteString(script); err != nil {
		return nil, errors.Wrap(err, "writing run script")
	}
	f.Close()

	common, err := w.DockerRunOpts(ctx, target)
	if err != nil {
		return nil, errors.Wrap(err, "generating run options")
	}

	opts := append([]string{
		"run",
		"--rm",
		"--init",
		"--workdir", target,
		"--mount", "type=bind,source=" + name + ",target=/run.sh,ro",
	}, common...)
	opts = append(opts, dockerWorkspaceImage, "sh", "/run.sh")

	out, err := exec.CommandContext(ctx, "docker", opts...).CombinedOutput()
	if err != nil {
		return out, errors.Wrapf(err, "Docker output:\n\n%s\n\n", string(out))
	}

	return out, nil
}

package campaigns

import (
	"bytes"
	"context"
	"os/exec"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/pkg/errors"
	"github.com/sourcegraph/src-cli/internal/campaigns/graphql"
	"github.com/sourcegraph/src-cli/internal/marionette"
	"google.golang.org/grpc"
)

const dockerMarionetteWorkspaceImage = "sourcegraph/src-marionette"

type dockerMarionetteWorkspaceCreator struct{}

var _ WorkspaceCreator = &dockerMarionetteWorkspaceCreator{}

func (wc *dockerMarionetteWorkspaceCreator) Create(ctx context.Context, repo *graphql.Repository, zip string) (Workspace, error) {
	volume, err := wc.createVolume(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "creating Docker volume")
	}

	w := &dockerMarionetteWorkspace{
		dir:    "/work",
		volume: volume,
	}

	common, err := w.DockerRunOpts(ctx, w.dir)
	if err != nil {
		return nil, errors.Wrap(err, "generating run options")
	}

	opts := append([]string{
		"run",
		"-d",
		"--rm",
		"--workdir", w.dir,
		"-p", "127.0.0.1::50051",
		"--mount", "type=bind,source=" + zip + ",target=/tmp/zip",
	}, common...)
	opts = append(
		opts,
		dockerMarionetteWorkspaceImage,
		"/usr/sbin/marionette",
		"-workspace", w.dir,
		"-network", "tcp",
		"-address", ":50051",
	)

	out, err := exec.CommandContext(ctx, "docker", opts...).CombinedOutput()
	if err != nil {
		return nil, errors.Wrap(err, "running Marionette container")
	}
	w.container = string(bytes.TrimSpace(out))

	// Figure out the port.
	out, err = exec.CommandContext(ctx, "docker", "port", w.container, "50051").CombinedOutput()
	if err != nil {
		return nil, errors.Wrap(err, "getting Marionette port")
	}

	// Connect.
	conn, err := grpc.Dial(string(bytes.TrimSpace(out)), grpc.WithInsecure(), grpc.WithBlock(), grpc.WithMaxMsgSize(2*1024*1024*1024))
	if err != nil {
		return nil, errors.Wrap(err, "connecting to Marionette")
	}
	w.conn = conn
	w.client = marionette.NewMarionetteClient(conn)

	// Unzip.
	if _, err := w.client.Unzip(ctx, &marionette.UnzipRequest{
		Path: "/tmp/zip",
	}); err != nil {
		return nil, errors.Wrap(err, "unzipping archive")
	}

	// Prepare.
	if _, err := w.client.Prepare(ctx, &empty.Empty{}); err != nil {
		return nil, errors.Wrap(err, "preparing workspace")
	}

	return w, nil
}

func (*dockerMarionetteWorkspaceCreator) DockerImages() []string {
	return []string{dockerMarionetteWorkspaceImage}
}

func (*dockerMarionetteWorkspaceCreator) createVolume(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "docker", "volume", "create").CombinedOutput()
	if err != nil {
		return "", err
	}

	return string(bytes.TrimSpace(out)), nil
}

type dockerMarionetteWorkspace struct {
	client    marionette.MarionetteClient
	conn      *grpc.ClientConn
	container string
	dir       string
	volume    string
}

func (w *dockerMarionetteWorkspace) DockerRunOpts(ctx context.Context, target string) ([]string, error) {
	return []string{
		"--mount", "type=volume,source=" + w.volume + ",target=" + target,
	}, nil
}

func (w *dockerMarionetteWorkspace) WorkDir() *string {
	return nil
}

func (w *dockerMarionetteWorkspace) Close(ctx context.Context) error {
	w.conn.Close()

	if err := exec.CommandContext(ctx, "docker", "stop", w.container).Run(); err != nil {
		return err
	}

	return exec.CommandContext(ctx, "docker", "volume", "rm", w.volume).Run()
}

func (w *dockerMarionetteWorkspace) Changes(ctx context.Context) (*StepChanges, error) {
	c, err := w.client.Changes(ctx, &empty.Empty{})
	if err != nil {
		return nil, err
	}

	return &StepChanges{
		Modified: c.Modified,
		Added:    c.Added,
		Deleted:  c.Deleted,
		Renamed:  c.Renamed,
	}, nil
}

func (w *dockerMarionetteWorkspace) Diff(ctx context.Context) ([]byte, error) {
	resp, err := w.client.Diff(ctx, &empty.Empty{})
	if err != nil {
		return nil, err
	}

	return resp.Diff, nil
}

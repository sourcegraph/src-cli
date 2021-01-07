package main

import (
	"context"
	"os/exec"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/sourcegraph/src-cli/internal/marionette"
)

type server struct {
	marionette.UnimplementedMarionetteServer

	workspace string
}

var _ marionette.MarionetteServer = &server{}

func (s *server) Prepare(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if _, err := s.command(ctx, "git", "init").run(); err != nil {
		return nil, err
	}
	if _, err := s.command(ctx, "git", "add", "--force", "--all").run(); err != nil {
		return nil, err
	}
	if _, err := s.command(ctx, "git", "commit", "--quiet", "--all", "--allow-empty", "-m", "src-action-exec").run(); err != nil {
		return nil, err
	}

	return &empty.Empty{}, nil
}

func (s *server) command(ctx context.Context, name string, arg ...string) command {
	cmd := exec.CommandContext(ctx, name, arg...)
	cmd.Dir = s.workspace

	return command{cmd}
}

type command struct {
	*exec.Cmd
}

func (c command) run() ([]byte, error) {
	// TODO: wrap execution errors in something we can marshal back to the
	// client.
	return c.CombinedOutput()
}

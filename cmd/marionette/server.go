package main

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/sourcegraph/src-cli/internal/campaigns"
	"github.com/sourcegraph/src-cli/internal/marionette"
)

type server struct {
	marionette.UnimplementedMarionetteServer

	workspace string
}

var _ marionette.MarionetteServer = &server{}

func (s *server) Changes(ctx context.Context, req *empty.Empty) (*marionette.ChangesResponse, error) {
	if _, err := s.command(ctx, "git", "add", "--all").run(); err != nil {
		return nil, err
	}

	out, err := s.command(ctx, "git", "status", "--porcelain").run()
	if err != nil {
		return nil, err
	}

	changes, err := parseGitStatus(out)
	if err != nil {
		return nil, err
	}

	return &marionette.ChangesResponse{
		Modified: changes.Modified,
		Added:    changes.Added,
		Deleted:  changes.Deleted,
		Renamed:  changes.Renamed,
	}, nil
}

func (s *server) Diff(ctx context.Context, req *empty.Empty) (*marionette.DiffResponse, error) {
	out, err := s.command(ctx, "git", "diff", "--cached", "--no-prefix", "--binary").run()
	if err != nil {
		return nil, err
	}

	return &marionette.DiffResponse{Diff: out}, nil
}

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

func (s *server) Unzip(ctx context.Context, req *marionette.UnzipRequest) (*empty.Empty, error) {
	if _, err := s.command(ctx, "unzip", req.Path).run(); err != nil {
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

// TODO: don't duplicate.
func parseGitStatus(out []byte) (campaigns.StepChanges, error) {
	result := campaigns.StepChanges{}

	stripped := strings.TrimSpace(string(out))
	if len(stripped) == 0 {
		return result, nil
	}

	for _, line := range strings.Split(stripped, "\n") {
		if len(line) < 4 {
			return result, fmt.Errorf("git status line has unrecognized format: %q", line)
		}

		file := line[3:]

		switch line[0] {
		case 'M':
			result.Modified = append(result.Modified, file)
		case 'A':
			result.Added = append(result.Added, file)
		case 'D':
			result.Deleted = append(result.Deleted, file)
		case 'R':
			files := strings.Split(file, " -> ")
			newFile := files[len(files)-1]
			result.Renamed = append(result.Renamed, newFile)
		}
	}

	return result, nil
}

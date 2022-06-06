package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"

	"github.com/sourcegraph/sourcegraph/lib/errors"

	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/batches/docker"
	"github.com/sourcegraph/src-cli/internal/batches/executor"
	"github.com/sourcegraph/src-cli/internal/batches/graphql"
	"github.com/sourcegraph/src-cli/internal/batches/service"
	"github.com/sourcegraph/src-cli/internal/batches/ui"
	"github.com/sourcegraph/src-cli/internal/batches/workspace"
	"github.com/sourcegraph/src-cli/internal/cmderrors"

	batcheslib "github.com/sourcegraph/sourcegraph/lib/batches"
)

func init() {
	usage := `
INTERNAL USE ONLY: 'src batch exec' executes the given raw batch spec in the given workspaces.

The input file contains a JSON dump of the WorkspacesExecutionInput struct in
github.com/sourcegraph/sourcegraph/lib/batches.

Usage:

    src batch exec -f FILE [command options]

Examples:

    $ src batch exec -f batch-spec-with-workspaces.json

`

	flagSet := flag.NewFlagSet("exec", flag.ExitOnError)
	flags := newBatchExecuteFlags(flagSet, true, batchDefaultCacheDir(), batchDefaultTempDirPrefix())

	handler := func(args []string) error {
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		if len(flagSet.Args()) != 0 {
			return cmderrors.Usage("additional arguments not allowed")
		}

		ctx, cancel := contextCancelOnInterrupt(context.Background())
		defer cancel()

		err := executeBatchSpecInWorkspaces(ctx, &ui.JSONLines{}, executeBatchSpecOpts{
			flags:  flags,
			client: &deadClient{},
		})
		if err != nil {
			return cmderrors.ExitCode(1, nil)
		}

		return nil
	}

	batchCommands = append(batchCommands, &command{
		flagSet: flagSet,
		handler: handler,
		usageFunc: func() {
			fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src batch %s':\n", flagSet.Name())
			flagSet.PrintDefaults()
			fmt.Println(usage)
		},
	})
}

func executeBatchSpecInWorkspaces(ctx context.Context, ui *ui.JSONLines, opts executeBatchSpecOpts) (err error) {
	defer func() {
		if err != nil {
			ui.ExecutionError(err)
		}
	}()

	if opts.flags.sourcegraphVersion == "" {
		return errors.New("missing sourcegraph-version flag")
	}

	svc := service.New(&service.Opts{
		AllowUnsupported: opts.flags.allowUnsupported,
		AllowIgnored:     opts.flags.allowIgnored,
		Client:           opts.client,
	})
	if err := svc.SetFeatureFlagsForRelease(opts.flags.sourcegraphVersion); err != nil {
		return err
	}

	// TODO: Do we need these checks in the controlled executors environment?
	if err := checkExecutable("git", "version"); err != nil {
		return err
	}
	if err := checkExecutable("docker", "version"); err != nil {
		return err
	}

	// Read the input file that contains the raw spec and the workspaces in
	// which to execute it.
	input, err := loadWorkspaceExecutionInput(opts.flags.file)
	if err != nil {
		return err
	}

	// Since we already know which workspace we want to execute the steps in,
	// we can convert it to a RepoWorkspace and build a task only for that one.
	tasks := svc.BuildTasks(ctx, &input.BatchChangeAttributes, []service.RepoWorkspace{convertWorkspace(input)})

	if len(tasks) != 1 {
		return errors.New("invalid input, didn't yield exactly one task")
	}

	task := tasks[0]

	if len(task.Steps) == 0 {
		return errors.New("invalid execution, no steps to process")
	}

	var (
		images map[string]docker.Image
	)

	{
		ui.PreparingContainerImages()
		images, err = svc.EnsureDockerImages(
			ctx, task.Steps, opts.flags.parallelism,
			ui.PreparingContainerImagesProgress,
		)
		if err != nil {
			return err
		}
		ui.PreparingContainerImagesSuccess()
	}

	// EXECUTION OF TASK
	coord := svc.NewCoordinator(executor.NewCoordinatorOpts{
		Creator: workspace.NewCreator(ctx, "executor", opts.flags.cacheDir, opts.flags.tempDir, images),
		// TODO: Shouldn't this be set always?
		// Answer: Not required. It's only used for the repo archive cache dir and we don't do repo archives here.
		CacheDir: opts.flags.cacheDir,
		// TODO: Make sure cache dir is set.
		Cache: &executor.ServerSideCache{CacheDir: opts.flags.cacheDir, Writer: ui},
		// We never want to skip errors on this level.
		SkipErrors: false,
		// This doesn't matter in SSBC, the disk is always wiped.
		// TODO: Or does it? What about dev.
		CleanArchives: false,
		Parallelism:   opts.flags.parallelism,
		// TODO: Should be slightly less than the executor timeout. Can we somehow read that?
		Timeout: opts.flags.timeout,
		// TODO: Not required?
		KeepLogs: opts.flags.keepLogs,
		// TODO: This is only used for a cidfile and for keep logs, should we remove it?
		TempDir: opts.flags.tempDir,
	}, true)

	// `src batch exec` uses server-side caching for changeset specs, so we
	// only need to call `CheckStepResultsCache` to make sure that per-step cache entries
	// are loaded and set on the tasks.
	if err := coord.CheckStepResultsCache(ctx, tasks); err != nil {
		return err
	}

	taskExecUI := ui.ExecutingTasks(*verbose, opts.flags.parallelism)
	err = coord.Execute(ctx, tasks, taskExecUI)
	if err != nil {
		taskExecUI.Failed(err)
		return err
	}

	taskExecUI.Success()
	return nil
}

func loadWorkspaceExecutionInput(file string) (batcheslib.WorkspacesExecutionInput, error) {
	var input batcheslib.WorkspacesExecutionInput

	f, err := batchOpenFileFlag(file)
	if err != nil {
		return input, err
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return input, errors.Wrap(err, "reading workspace execution input file")
	}

	if err := json.Unmarshal(data, &input); err != nil {
		return input, errors.Wrap(err, "unmarshaling workspace execution input file")
	}

	return input, nil
}

func convertWorkspace(w batcheslib.WorkspacesExecutionInput) service.RepoWorkspace {
	fileMatches := make(map[string]bool)
	for _, path := range w.SearchResultPaths {
		fileMatches[path] = true
	}
	return service.RepoWorkspace{
		Repo: &graphql.Repository{
			ID:   w.Repository.ID,
			Name: w.Repository.Name,
			Branch: graphql.Branch{
				Name: w.Branch.Name,
				Target: graphql.Target{
					OID: w.Branch.Target.OID,
				},
			},
			Commit:      graphql.Target{OID: w.Branch.Target.OID},
			FileMatches: fileMatches,
		},
		Path:               w.Path,
		Steps:              w.Steps,
		OnlyFetchWorkspace: w.OnlyFetchWorkspace,
	}
}

type deadClient struct{}

var _ api.Client = &deadClient{}

func (c *deadClient) NewQuery(query string) api.Request {
	panic("dead client invoked")
}
func (c *deadClient) NewRequest(query string, vars map[string]interface{}) api.Request {
	panic("dead client invoked")
}
func (c *deadClient) NewGzippedRequest(query string, vars map[string]interface{}) api.Request {
	panic("dead client invoked")
}
func (c *deadClient) NewGzippedQuery(query string) api.Request {
	panic("dead client invoked")
}
func (c *deadClient) NewHTTPRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	panic("dead client invoked")
}
func (c *deadClient) Do(req *http.Request) (*http.Response, error) {
	panic("dead client invoked")
}

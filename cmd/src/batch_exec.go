package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/cockroachdb/errors"
	"github.com/hashicorp/go-multierror"
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
INTERNAL USER ONLY: 'src batch exec' executes the given raw batch spec in the given workspaces.

Usage:

    src batch exec -f batch-spec-with-workspaces.json

Examples:

    $ src batch apply -f batch-spec-with-workspaces.json
`

	flagSet := flag.NewFlagSet("exec", flag.ExitOnError)
	flags := newBatchExecuteFlags(flagSet, batchDefaultCacheDir(), batchDefaultTempDirPrefix())

	handler := func(args []string) error {
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		if len(flagSet.Args()) != 0 {
			return cmderrors.Usage("additional arguments not allowed")
		}

		// Only text output is allowed
		flags.textOnly = true

		ctx, cancel := contextCancelOnInterrupt(context.Background())
		defer cancel()

		err := executeBatchSpecInWorkspaces(ctx, executeBatchSpecOpts{
			flags:  flags,
			client: cfg.apiClient(flags.api, flagSet.Output()),
			ui:     &ui.JSONLines{},
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

type WorkspacesResult struct {
	RawSpec    string                  `json:"rawSpec"`
	Workspaces SerializeableWorkspaces `json:"workspaces"`
}

type SerializeableWorkspace struct {
	Repository struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"repository"`
	Branch struct {
		Name   string `json:"name"`
		Target struct {
			OID string `json:"oid"`
		} `json:"target"`
	} `json:"branch"`
	Path               string            `json:"path"`
	OnlyFetchWorkspace bool              `json:"onlyFetchWorkspace"`
	Steps              []batcheslib.Step `json:"steps"`
	SearchResultPaths  []string          `json:"searchResultPaths"`
}

type SerializeableWorkspaces []*SerializeableWorkspace

func (ws SerializeableWorkspaces) ToRepoWorkspaces() []service.RepoWorkspace {
	workspaces := make([]service.RepoWorkspace, 0, len(ws))
	for _, w := range ws {
		fileMatches := make(map[string]bool)
		for _, path := range w.SearchResultPaths {
			fileMatches[path] = true
		}
		workspaces = append(workspaces, service.RepoWorkspace{
			Repo: &graphql.Repository{
				ID:   w.Repository.ID,
				Name: w.Repository.Name,
				Branch: graphql.Branch{
					Name: w.Branch.Name,
					Target: graphql.Target{
						OID: w.Branch.Target.OID,
					},
				},
				ExternalRepository: struct{ ServiceType string }{ServiceType: "github"},
				DefaultBranch: &graphql.Branch{
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
		})
	}
	return workspaces
}

func executeBatchSpecInWorkspaces(ctx context.Context, opts executeBatchSpecOpts) (err error) {
	defer func() {
		if err != nil {
			opts.ui.ExecutionError(err)
		}
	}()

	svc := service.New(&service.Opts{
		AllowUnsupported: opts.flags.allowUnsupported,
		AllowIgnored:     opts.flags.allowIgnored,
		Client:           opts.client,
	})
	if err := svc.DetermineFeatureFlags(ctx); err != nil {
		return err
	}

	if err := checkExecutable("git", "version"); err != nil {
		return err
	}
	if err := checkExecutable("docker", "version"); err != nil {
		return err
	}

	f, err := batchOpenFileFlag(&opts.flags.file)
	if err != nil {
		return err
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return errors.Wrap(err, "reading batch spec")
	}

	var input WorkspacesResult
	if err := json.Unmarshal(data, &input); err != nil {
		return errors.Wrap(err, "unmarshaling input file")
	}

	// Parse flags and build up our service and executor options.
	opts.ui.ParsingBatchSpec()
	batchSpec, err := svc.ParseBatchSpec([]byte(input.RawSpec))
	if err != nil {
		if merr, ok := err.(*multierror.Error); ok {
			opts.ui.ParsingBatchSpecFailure(merr)
			return cmderrors.ExitCode(2, nil)
		} else {
			// This shouldn't happen; let's just punt and let the normal
			// rendering occur.
			return err
		}
	}
	opts.ui.ParsingBatchSpecSuccess()

	opts.ui.PreparingContainerImages()
	images, err := svc.EnsureDockerImages(ctx, batchSpec, opts.ui.PreparingContainerImagesProgress)
	if err != nil {
		return err
	}
	opts.ui.PreparingContainerImagesSuccess()

	opts.ui.DeterminingWorkspaceCreatorType()
	workspaceCreator := workspace.NewCreator(ctx, opts.flags.workspace, opts.flags.cacheDir, opts.flags.tempDir, images)
	if workspaceCreator.Type() == workspace.CreatorTypeVolume {
		_, err = svc.EnsureImage(ctx, workspace.DockerVolumeWorkspaceImage)
		if err != nil {
			return err
		}
	}
	opts.ui.DeterminingWorkspaceCreatorTypeSuccess(workspaceCreator.Type())

	// EXECUTION OF TASKS
	coord := svc.NewCoordinator(executor.NewCoordinatorOpts{
		Creator:       workspaceCreator,
		CacheDir:      opts.flags.cacheDir,
		ClearCache:    opts.flags.clearCache,
		SkipErrors:    opts.flags.skipErrors,
		CleanArchives: opts.flags.cleanArchives,
		Parallelism:   opts.flags.parallelism,
		Timeout:       opts.flags.timeout,
		KeepLogs:      opts.flags.keepLogs,
		TempDir:       opts.flags.tempDir,
	})

	opts.ui.CheckingCache()
	tasks := svc.BuildTasks(ctx, batchSpec, input.Workspaces.ToRepoWorkspaces())
	uncachedTasks, cachedSpecs, err := coord.CheckCache(ctx, tasks)
	if err != nil {
		return err
	}
	opts.ui.CheckingCacheSuccess(len(cachedSpecs), len(uncachedTasks))

	taskExecUI := opts.ui.ExecutingTasks(*verbose, opts.flags.parallelism)
	freshSpecs, _, err := coord.Execute(ctx, uncachedTasks, batchSpec, taskExecUI)
	if err != nil && !opts.flags.skipErrors {
		return err
	}
	taskExecUI.Success()
	if err != nil && opts.flags.skipErrors {
		opts.ui.ExecutingTasksSkippingErrors(err)
	}

	specs := append(cachedSpecs, freshSpecs...)

	ids := make([]graphql.ChangesetSpecID, len(specs))

	if len(specs) > 0 {
		opts.ui.UploadingChangesetSpecs(len(specs))

		for i, spec := range specs {
			id, err := svc.CreateChangesetSpec(ctx, spec)
			if err != nil {
				return err
			}
			ids[i] = id
			opts.ui.UploadingChangesetSpecsProgress(i+1, len(specs))
		}

		opts.ui.UploadingChangesetSpecsSuccess(ids)
	}

	return nil
}

package executor

import (
	"context"
	"io"
	"reflect"
	"strconv"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hashicorp/go-multierror"
	batcheslib "github.com/sourcegraph/sourcegraph/lib/batches"
	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/batches"
	"github.com/sourcegraph/src-cli/internal/batches/docker"
	"github.com/sourcegraph/src-cli/internal/batches/graphql"
	"github.com/sourcegraph/src-cli/internal/batches/log"
	"github.com/sourcegraph/src-cli/internal/batches/workspace"
)

type taskExecutor interface {
	Start(context.Context, []*Task, TaskExecutionUI)
	Wait(context.Context) ([]taskResult, error)
}

// Coordinates coordinates the execution of Tasks. It makes use of an executor,
// checks the ExecutionCache whether execution is necessary, builds
// batcheslib.ChangesetSpecs out of the executionResults.
type Coordinator struct {
	opts NewCoordinatorOpts

	cache      ExecutionCache
	exec       taskExecutor
	logManager log.LogManager
}

type repoNameResolver func(ctx context.Context, name string) (*graphql.Repository, error)
type imageEnsurer func(ctx context.Context, name string) (docker.Image, error)

type NewCoordinatorOpts struct {
	// Dependencies
	ResolveRepoName repoNameResolver
	EnsureImage     imageEnsurer
	Creator         workspace.Creator
	Client          api.Client

	// Everything that follows are either command-line flags or features.

	// TODO: We could probably have a wrapper around flags and features,
	// something like ExecutionArgs, that we can pass around
	CacheDir   string
	ClearCache bool
	SkipErrors bool

	// Used by createChangesetSpecs
	Features batches.FeatureFlags

	CleanArchives bool
	Parallelism   int
	Timeout       time.Duration
	KeepLogs      bool
	TempDir       string
}

func NewCoordinator(opts NewCoordinatorOpts) *Coordinator {
	cache := NewCache(opts.CacheDir)
	logManager := log.NewManager(opts.TempDir, opts.KeepLogs)

	exec := newExecutor(newExecutorOpts{
		Fetcher:     batches.NewRepoFetcher(opts.Client, opts.CacheDir, opts.CleanArchives),
		EnsureImage: opts.EnsureImage,
		Creator:     opts.Creator,
		Logger:      logManager,

		AutoAuthorDetails: opts.Features.IncludeAutoAuthorDetails,
		Parallelism:       opts.Parallelism,
		Timeout:           opts.Timeout,
		TempDir:           opts.TempDir,
	})

	return &Coordinator{
		opts: opts,

		cache:      cache,
		exec:       exec,
		logManager: logManager,
	}
}

// CheckCache checks whether the internal ExecutionCache contains
// ChangesetSpecs for the given Tasks. If cached ChangesetSpecs exist, those
// are returned, otherwise the Task, to be executed later.
func (c *Coordinator) CheckCache(ctx context.Context, tasks []*Task) (uncached []*Task, specs []*batcheslib.ChangesetSpec, err error) {
	for _, t := range tasks {
		cachedSpecs, found, err := c.checkCacheForTask(ctx, t)
		if err != nil {
			return nil, nil, err
		}

		if !found {
			uncached = append(uncached, t)
			continue
		}

		specs = append(specs, cachedSpecs...)
	}

	return uncached, specs, nil
}

func (c *Coordinator) checkCacheForTask(ctx context.Context, task *Task) (specs []*batcheslib.ChangesetSpec, found bool, err error) {
	// Check if the task is cached.
	cacheKey := task.cacheKey()
	if c.opts.ClearCache {
		if err := c.cache.Clear(ctx, cacheKey); err != nil {
			return specs, false, errors.Wrapf(err, "clearing cache for %q", task.Repository.Name)
		}

		return specs, false, nil
	}

	var result executionResult
	result, found, err = c.cache.Get(ctx, cacheKey)
	if err != nil {
		return specs, false, errors.Wrapf(err, "checking cache for %q", task.Repository.Name)
	}

	if !found {
		return specs, false, nil
	}

	// If the cached result resulted in an empty diff, we don't need to
	// add it to the list of specs that are displayed to the user and
	// send to the server. Instead, we can just report that the task is
	// complete and move on.
	if result.Diff == "" {
		return specs, true, nil
	}

	specs, err = createChangesetSpecs(task, result, c.opts.Features)
	if err != nil {
		return specs, false, err
	}

	return specs, true, nil
}

func (c *Coordinator) setCachedStepResults(ctx context.Context, task *Task) error {
	// We start at the back so that we can find the _last_ cached step,
	// then restart execution on the following step.
	for i := len(task.Steps) - 1; i > -1; i-- {
		key := StepsCacheKey{Task: task, StepIndex: i}

		// If we need to clear the cache, we optimistically try this for every
		// step.
		if c.opts.ClearCache {
			if err := c.cache.Clear(ctx, key); err != nil {
				return errors.Wrapf(err, "clearing cache for step %d in %q", i, task.Repository.Name)
			}
		} else {
			result, found, err := c.cache.GetStepResult(ctx, key)
			if err != nil {
				return errors.Wrapf(err, "checking for cached diff for step %d", i)
			}

			// Found a cached result, we're done
			if found {
				task.CachedResultFound = true
				task.CachedResult = result
				return nil
			}
		}
	}

	return nil
}

func (c *Coordinator) cacheAndBuildSpec(ctx context.Context, taskResult taskResult, ui TaskExecutionUI) ([]*batcheslib.ChangesetSpec, error) {
	// Add to the cache, even if no diff was produced.
	cacheKey := taskResult.task.cacheKey()
	if err := c.cache.Set(ctx, cacheKey, taskResult.result); err != nil {
		return nil, errors.Wrapf(err, "caching result for %q", taskResult.task.Repository.Name)
	}

	// Save the per-step results
	for _, stepResult := range taskResult.stepResults {
		key := StepsCacheKey{Task: taskResult.task, StepIndex: stepResult.StepIndex}
		if err := c.cache.SetStepResult(ctx, key, stepResult); err != nil {
			return nil, errors.Wrapf(err, "caching result for step %d in %q", stepResult.StepIndex, taskResult.task.Repository.Name)
		}
	}

	// If the steps didn't result in any diff, we don't need to create a
	// changeset spec that's displayed to the user and send to the server.
	if taskResult.result.Diff == "" {
		return nil, nil
	}

	// Build the changeset specs.
	specs, err := createChangesetSpecs(taskResult.task, taskResult.result, c.opts.Features)
	if err != nil {
		return nil, err
	}

	ui.TaskChangesetSpecsBuilt(taskResult.task, specs)
	return specs, nil
}

type TaskExecutionUI interface {
	Start([]*Task)
	Success()

	TaskStarted(*Task)
	TaskFinished(*Task, error)

	TaskChangesetSpecsBuilt(*Task, []*batcheslib.ChangesetSpec)

	// TODO: This should be split up into methods that are more specific.
	TaskCurrentlyExecuting(*Task, string)

	StepStdoutWriter(context.Context, *Task, int) io.WriteCloser
	StepStderrWriter(context.Context, *Task, int) io.WriteCloser
}

// Execute executes the given Tasks and the importChangeset statements in the
// given spec. It regularly calls the executionProgressPrinter with the
// current TaskStatuses.
func (c *Coordinator) Execute(ctx context.Context, tasks []*Task, spec *batcheslib.BatchSpec, ui TaskExecutionUI) ([]*batcheslib.ChangesetSpec, []string, error) {
	var (
		specs []*batcheslib.ChangesetSpec
		errs  *multierror.Error
	)

	// If we are here, that means we didn't find anything in the cache for the
	// complete task. So, what if we have cached results for the steps?
	for _, t := range tasks {
		if err := c.setCachedStepResults(ctx, t); err != nil {
			return nil, nil, err
		}
	}

	ui.Start(tasks)

	// Run executor
	c.exec.Start(ctx, tasks, ui)
	results, err := c.exec.Wait(ctx)
	if err != nil {
		if c.opts.SkipErrors {
			errs = multierror.Append(errs, err)
		} else {
			return nil, nil, err
		}
	}

	// Write results to cache, build ChangesetSpecs if possible and add to list.
	for _, taskResult := range results {
		taskSpecs, err := c.cacheAndBuildSpec(ctx, taskResult, ui)
		if err != nil {
			return nil, nil, err
		}

		specs = append(specs, taskSpecs...)
	}

	// Add external changeset specs.
	for _, ic := range spec.ImportChangesets {
		repo, err := c.opts.ResolveRepoName(ctx, ic.Repository)
		if err != nil {
			wrapped := errors.Wrapf(err, "resolving repository name %q", ic.Repository)
			if c.opts.SkipErrors {
				errs = multierror.Append(errs, wrapped)
				continue
			} else {
				return nil, nil, wrapped
			}
		}

		for _, id := range ic.ExternalIDs {
			var sid string

			switch tid := id.(type) {
			case string:
				sid = tid
			case int, int8, int16, int32, int64:
				sid = strconv.FormatInt(reflect.ValueOf(id).Int(), 10)
			case uint, uint8, uint16, uint32, uint64:
				sid = strconv.FormatUint(reflect.ValueOf(id).Uint(), 10)
			case float32:
				sid = strconv.FormatFloat(float64(tid), 'f', -1, 32)
			case float64:
				sid = strconv.FormatFloat(tid, 'f', -1, 64)
			default:
				return nil, nil, errors.Errorf("cannot convert value of type %T into a valid external ID: expected string or int", id)
			}

			specs = append(specs, &batcheslib.ChangesetSpec{
				BaseRepository: repo.ID,

				ExternalID: sid,
			})
		}
	}

	return specs, c.logManager.LogFiles(), errs.ErrorOrNil()
}

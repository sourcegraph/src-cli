package executor

import (
	"context"
	"sync"

	"github.com/sourcegraph/sourcegraph/lib/errors"

	batcheslib "github.com/sourcegraph/sourcegraph/lib/batches"
	"github.com/sourcegraph/sourcegraph/lib/batches/execution"
	"github.com/sourcegraph/sourcegraph/lib/batches/execution/cache"

	"github.com/sourcegraph/src-cli/internal/batches/log"
)

type taskExecutor interface {
	Start(context.Context, []*Task, TaskExecutionUI)
	Wait() ([]taskResult, error)
}

// Coordinator coordinates the execution of Tasks. It makes use of an executor,
// checks the ExecutionCache whether execution is necessary, and builds
// batcheslib.ChangesetSpecs out of the executionResults.
type Coordinator struct {
	opts NewCoordinatorOpts

	exec taskExecutor

	// cacheMutex protects concurrent access to the cache
	cacheMutex sync.Mutex
	// specsMutex protects access to the changesets specs during build
	specsMutex sync.Mutex
}

type NewCoordinatorOpts struct {
	ExecOpts NewExecutorOpts

	Cache       cache.Cache
	Logger      log.LogManager
	GlobalEnv   []string
	BinaryDiffs bool

	IsRemote bool
}

func NewCoordinator(opts NewCoordinatorOpts) *Coordinator {
	return &Coordinator{
		opts: opts,
		exec: NewExecutor(opts.ExecOpts),
	}
}

// CheckCache checks whether the internal ExecutionCache contains
// ChangesetSpecs for the given Tasks. If cached ChangesetSpecs exist, those
// are returned, otherwise the Task, to be executed later.
func (c *Coordinator) CheckCache(ctx context.Context, batchSpec *batcheslib.BatchSpec, tasks []*Task) (uncached []*Task, specs []*batcheslib.ChangesetSpec, err error) {
	for _, t := range tasks {
		cachedSpecs, found, err := c.checkCacheForTask(ctx, batchSpec, t)
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

func (c *Coordinator) ClearCache(ctx context.Context, tasks []*Task) error {
	// Lock to protect cache operations from race conditions
	c.cacheMutex.Lock()
	defer c.cacheMutex.Unlock()

	for _, task := range tasks {
		for i := len(task.Steps) - 1; i > -1; i-- {
			key := task.CacheKey(c.opts.GlobalEnv, c.opts.ExecOpts.WorkingDirectory, i)
			if err := c.opts.Cache.Clear(ctx, key); err != nil {
				return errors.Wrapf(err, "clearing cache for step %d in %q", i, task.Repository.Name)
			}
		}
	}
	return nil
}

func (c *Coordinator) checkCacheForTask(ctx context.Context, batchSpec *batcheslib.BatchSpec, task *Task) (specs []*batcheslib.ChangesetSpec, found bool, err error) {
	if err := c.loadCachedStepResults(ctx, task, c.opts.GlobalEnv); err != nil {
		return specs, false, err
	}

	// If we have cached results and don't need to execute any more steps,
	// we build changeset specs and return.
	// TODO: This doesn't consider skipped steps.
	if task.CachedStepResultFound && task.CachedStepResult.StepIndex == len(task.Steps)-1 {
		// Lock to protect cache operations and ensure atomicity
		c.cacheMutex.Lock()

		// If the cached result resulted in an empty diff, we don't need to
		// add it to the list of specs that are displayed to the user and
		// send to the server. Instead, we can just report that the task is
		// complete and move on.
		if len(task.CachedStepResult.Diff) == 0 {
			c.cacheMutex.Unlock()
			// Force re-execution by clearing cache for this task
			key := task.CacheKey(c.opts.GlobalEnv, c.opts.ExecOpts.WorkingDirectory, task.CachedStepResult.StepIndex)
			c.opts.Cache.Clear(ctx, key)
			return specs, false, nil // Return false to force re-execution
		}

		specs, err = c.buildChangesetSpecs(task, batchSpec, task.CachedStepResult)
		c.cacheMutex.Unlock()
		return specs, true, err
	}

	return specs, false, nil
}

func (c *Coordinator) buildChangesetSpecs(task *Task, batchSpec *batcheslib.BatchSpec, result execution.AfterStepResult) ([]*batcheslib.ChangesetSpec, error) {
	// Lock to protect spec building and ensure atomicity
	c.specsMutex.Lock()
	defer c.specsMutex.Unlock()

	// Validate diff is not empty
	if len(result.Diff) == 0 {
		return nil, errors.New("diff was empty during changeset spec creation")
	}

	version := 1
	if c.opts.BinaryDiffs {
		version = 2
	}
	input := &batcheslib.ChangesetSpecInput{
		Repository: batcheslib.Repository{
			ID:          task.Repository.ID,
			Name:        task.Repository.Name,
			FileMatches: task.Repository.SortedFileMatches(),
			BaseRef:     task.Repository.BaseRef(),
			BaseRev:     task.Repository.Rev(),
		},
		Path:                  task.Path,
		BatchChangeAttributes: task.BatchChangeAttributes,
		Template:              batchSpec.ChangesetTemplate,
		TransformChanges:      batchSpec.TransformChanges,

		Result: execution.AfterStepResult{
			Version:      version,
			Diff:         result.Diff,
			ChangedFiles: result.ChangedFiles,
			Outputs:      result.Outputs,
		},
	}

	return batcheslib.BuildChangesetSpecs(input, c.opts.BinaryDiffs, nil)
}

func (c *Coordinator) loadCachedStepResults(ctx context.Context, task *Task, globalEnv []string) error {
	// Lock to protect cache operations from race conditions
	c.cacheMutex.Lock()
	defer c.cacheMutex.Unlock()

	// We start at the back so that we can find the _last_ cached step,
	// then restart execution on the following step.
	for i := len(task.Steps) - 1; i > -1; i-- {
		key := task.CacheKey(globalEnv, c.opts.ExecOpts.WorkingDirectory, i)

		result, found, err := c.opts.Cache.Get(ctx, key)
		if err != nil {
			return errors.Wrapf(err, "checking for cached diff for step %d", i)
		}

		// Found a cached result, we're done.
		if found {
			task.CachedStepResultFound = true
			task.CachedStepResult = result
			return nil
		}
	}

	return nil
}

func (c *Coordinator) buildSpecs(ctx context.Context, batchSpec *batcheslib.BatchSpec, taskResult taskResult, ui TaskExecutionUI) ([]*batcheslib.ChangesetSpec, error) {
	// Lock to protect spec building from race conditions
	c.specsMutex.Lock()
	defer c.specsMutex.Unlock()

	if len(taskResult.stepResults) == 0 {
		return nil, nil
	}

	lastStepResult := taskResult.stepResults[len(taskResult.stepResults)-1]

	// If the steps didn't result in any diff, we don't need to create a
	// changeset spec that's displayed to the user and send to the server.
	if len(lastStepResult.Diff) == 0 {
		return nil, nil
	}

	// Build the changeset specs.
	specs, err := c.buildChangesetSpecs(taskResult.task, batchSpec, lastStepResult)
	if err != nil {
		return nil, err
	}

	ui.TaskChangesetSpecsBuilt(taskResult.task, specs)
	return specs, nil
}

// ExecuteAndBuildSpecs executes the given tasks and builds changeset specs for the results.
// It calls the ui on updates.
func (c *Coordinator) ExecuteAndBuildSpecs(ctx context.Context, batchSpec *batcheslib.BatchSpec, tasks []*Task, ui TaskExecutionUI) ([]*batcheslib.ChangesetSpec, []string, error) {
	ui.Start(tasks)

	// Run executor.
	c.exec.Start(ctx, tasks, ui)
	results, errs := c.exec.Wait()

	// Create a copy of results to safely iterate over during cache operations
	resultsCopy := make([]taskResult, len(results))
	copy(resultsCopy, results)

	// Write all step cache results to the cache.
	// Lock to protect cache operations from race conditions
	c.cacheMutex.Lock()
	for _, res := range resultsCopy {
		for _, stepRes := range res.stepResults {
			cacheKey := res.task.CacheKey(c.opts.GlobalEnv, c.opts.ExecOpts.WorkingDirectory, stepRes.StepIndex)
			if err := c.opts.Cache.Set(ctx, cacheKey, stepRes); err != nil {
				c.cacheMutex.Unlock() // Release the lock before returning
				return nil, nil, errors.Wrapf(err, "caching result for step %d", stepRes.StepIndex)
			}
		}
	}
	c.cacheMutex.Unlock()

	var specs []*batcheslib.ChangesetSpec

	// Build ChangesetSpecs if possible and add to list.
	// Using the copy of results to avoid race conditions
	for _, taskResult := range resultsCopy {
		// Don't build changeset specs for failed workspaces.
		if taskResult.err != nil {
			continue
		}

		taskSpecs, err := c.buildSpecs(ctx, batchSpec, taskResult, ui)
		if err != nil {
			return nil, nil, err
		}

		specs = append(specs, taskSpecs...)
	}

	return specs, c.opts.Logger.LogFiles(), errs
}

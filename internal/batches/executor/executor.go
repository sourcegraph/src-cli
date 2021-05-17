package executor

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
	"time"

	"github.com/neelance/parallel"
	"github.com/pkg/errors"
	"github.com/sourcegraph/src-cli/internal/batches"
	"github.com/sourcegraph/src-cli/internal/batches/log"
	"github.com/sourcegraph/src-cli/internal/batches/workspace"
)

type TaskExecutionErr struct {
	Err        error
	Logfile    string
	Repository string
}

func (e TaskExecutionErr) Cause() error {
	return e.Err
}

func (e TaskExecutionErr) Error() string {
	return fmt.Sprintf(
		"execution in %s failed: %s (see %s for details)",
		e.Repository,
		e.Err,
		e.Logfile,
	)
}

func (e TaskExecutionErr) StatusText() string {
	if stepErr, ok := e.Err.(stepFailedErr); ok {
		return stepErr.SingleLineError()
	}
	return e.Err.Error()
}

type newExecutorOpts struct {
	// Dependencies
	Cache   ExecutionCache
	Creator workspace.Creator
	Status  *TaskStatusCollection
	Fetcher batches.RepoFetcher
	Logger  *log.Manager

	// Config
	AutoAuthorDetails bool
	Parallelism       int
	Timeout           time.Duration
	TempDir           string
}

type executor struct {
	opts newExecutorOpts

	par           *parallel.Run
	doneEnqueuing chan struct{}

	specs   []*batches.ChangesetSpec
	specsMu sync.Mutex
}

func newExecutor(opts newExecutorOpts) *executor {
	return &executor{
		opts: opts,

		doneEnqueuing: make(chan struct{}),
		par:           parallel.NewRun(opts.Parallelism),
	}
}

func (x *executor) LogFiles() []string {
	return x.opts.Logger.LogFiles()
}

func (x *executor) Start(ctx context.Context, tasks []*Task) {
	defer func() { close(x.doneEnqueuing) }()

	for _, task := range tasks {
		select {
		case <-ctx.Done():
			return
		default:
		}

		x.par.Acquire()

		go func(task *Task) {
			defer x.par.Release()

			select {
			case <-ctx.Done():
				return
			default:
				err := x.do(ctx, task)
				if err != nil {
					x.par.Error(err)
				}
			}
		}(task)
	}
}

func (x *executor) Wait(ctx context.Context) ([]*batches.ChangesetSpec, error) {
	<-x.doneEnqueuing

	result := make(chan error, 1)

	go func(ch chan error) {
		ch <- x.par.Wait()
	}(result)

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-result:
		close(result)
		if err != nil {
			return x.specs, err
		}
	}

	return x.specs, nil
}

func (x *executor) do(ctx context.Context, task *Task) (err error) {
	// Ensure that the status is updated when we're done.
	defer func() {
		x.opts.Status.Update(task, func(status *TaskStatus) {
			status.FinishedAt = time.Now()
			status.CurrentlyExecuting = ""
			status.Err = err
		})
	}()

	// We're away!
	x.opts.Status.Update(task, func(status *TaskStatus) {
		status.StartedAt = time.Now()
	})

	// It isn't, so let's get ready to run the task. First, let's set up our
	// logging.
	log, err := x.opts.Logger.AddTask(task.Repository.SlugForPath(task.Path))
	if err != nil {
		err = errors.Wrap(err, "creating log file")
		return
	}
	defer func() {
		if err != nil {
			err = TaskExecutionErr{
				Err:        err,
				Logfile:    log.Path(),
				Repository: task.Repository.Name,
			}
			log.MarkErrored()
		}
		log.Close()
	}()

	// Now checkout the archive
	task.Archive = x.opts.Fetcher.Checkout(task.Repository, task.ArchivePathToFetch())

	// Set up our timeout.
	runCtx, cancel := context.WithTimeout(ctx, x.opts.Timeout)
	defer cancel()

	// Actually execute the steps.
	opts := &executionOpts{
		archive:               task.Archive,
		wc:                    x.opts.Creator,
		batchChangeAttributes: task.BatchChangeAttributes,
		repo:                  task.Repository,
		path:                  task.Path,
		steps:                 task.Steps,
		logger:                log,
		tempDir:               x.opts.TempDir,
		reportProgress: func(currentlyExecuting string) {
			x.opts.Status.Update(task, func(status *TaskStatus) {
				status.CurrentlyExecuting = currentlyExecuting
			})
		},
	}

	result, err := runSteps(runCtx, opts)
	if err != nil {
		if reachedTimeout(runCtx, err) {
			err = &errTimeoutReached{timeout: x.opts.Timeout}
		}
		return
	}

	// Check if the task is cached.
	cacheKey := task.cacheKey()

	// Add to the cache. We don't use runCtx here because we want to write to
	// the cache even if we've now reached the timeout.
	if err = x.opts.Cache.Set(ctx, cacheKey, result); err != nil {
		err = errors.Wrapf(err, "caching result for %q", task.Repository.Name)
	}

	// If the steps didn't result in any diff, we don't need to add it to the
	// list of specs that are displayed to the user and send to the server.
	if result.Diff == "" {
		return
	}

	// Build the changeset specs.
	specs, err := createChangesetSpecs(task, result, x.opts.AutoAuthorDetails)
	if err != nil {
		return err
	}

	x.opts.Status.Update(task, func(status *TaskStatus) {
		status.ChangesetSpecs = specs
	})

	if err := x.addCompletedSpecs(specs); err != nil {
		return err
	}

	return
}

func (x *executor) addCompletedSpecs(specs []*batches.ChangesetSpec) error {
	x.specsMu.Lock()
	defer x.specsMu.Unlock()

	x.specs = append(x.specs, specs...)
	return nil
}

type errTimeoutReached struct{ timeout time.Duration }

func (e *errTimeoutReached) Error() string {
	return fmt.Sprintf("Timeout reached. Execution took longer than %s.", e.timeout)
}

func reachedTimeout(cmdCtx context.Context, err error) bool {
	if ee, ok := errors.Cause(err).(*exec.ExitError); ok {
		if ee.String() == "signal: killed" && cmdCtx.Err() == context.DeadlineExceeded {
			return true
		}
	}

	return errors.Is(errors.Cause(err), context.DeadlineExceeded)
}

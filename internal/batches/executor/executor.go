package executor

import (
	"context"
	"fmt"
	"github.com/sourcegraph/conc/pool"
	"time"

	"github.com/sourcegraph/sourcegraph/lib/errors"

	"github.com/sourcegraph/src-cli/internal/batches/docker"
	"github.com/sourcegraph/src-cli/internal/batches/log"
	"github.com/sourcegraph/src-cli/internal/batches/repozip"
	"github.com/sourcegraph/src-cli/internal/batches/util"
	"github.com/sourcegraph/src-cli/internal/batches/workspace"

	"github.com/sourcegraph/sourcegraph/lib/batches/execution"
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

// taskResult is a combination of a Task and the result of its execution.
type taskResult struct {
	task        *Task
	stepResults []execution.AfterStepResult
	err         error
}

type imageEnsurer func(ctx context.Context, name string) (docker.Image, error)

type NewExecutorOpts struct {
	// Dependencies
	Creator             workspace.Creator
	RepoArchiveRegistry repozip.ArchiveRegistry
	EnsureImage         imageEnsurer
	Logger              log.LogManager

	// Config
	Parallelism      int
	Timeout          time.Duration
	WorkingDirectory string
	TempDir          string
	IsRemote         bool
	GlobalEnv        []string
	ForceRoot        bool
	FailFast         bool

	BinaryDiffs bool
	Context     context.Context
}

type executor struct {
	opts NewExecutorOpts

	workPool      *pool.ResultContextPool[*taskResult]
	doneEnqueuing chan struct{}

	results []taskResult
}

func NewExecutor(opts NewExecutorOpts) *executor {
	p := pool.NewWithResults[*taskResult]().WithMaxGoroutines(opts.Parallelism).WithContext(opts.Context)
	if opts.FailFast {
		p = p.WithCancelOnError()
	}
	return &executor{
		opts: opts,

		doneEnqueuing: make(chan struct{}),
		workPool:      p,
	}
}

// Start starts the execution of the given Tasks in goroutines, calling the
// given taskStatusHandler to update the progress of the tasks.
func (x *executor) Start(tasks []*Task, ui TaskExecutionUI) {
	defer func() { close(x.doneEnqueuing) }()

	for _, task := range tasks {
		select {
		case <-x.opts.Context.Done():
			return
		default:
		}

		t := task
		x.workPool.Go(func(c context.Context) (*taskResult, error) {
			return x.do(c, t, ui)
		})
	}
}

// Wait blocks until all Tasks enqueued with Start have been executed.
func (x *executor) Wait() ([]taskResult, error) {
	<-x.doneEnqueuing

	r, err := x.workPool.Wait()
	results := make([]taskResult, len(r))
	for i, r := range r {
		if r == nil {
			results[i] = taskResult{
				task:        nil,
				stepResults: nil,
				err:         err,
			}
		} else {
			results[i] = *r
		}
	}
	return results, err
}

func (x *executor) do(ctx context.Context, task *Task, ui TaskExecutionUI) (result *taskResult, err error) {
	// Ensure that the status is updated when we're done.
	defer func() {
		ui.TaskFinished(task, err)
	}()

	// We're away!
	ui.TaskStarted(task)

	// Let's set up our logging.
	l, err := x.opts.Logger.AddTask(util.SlugForPathInRepo(task.Repository.Name, task.Repository.Rev(), task.Path))
	if err != nil {
		return nil, errors.Wrap(err, "creating log file")
	}
	defer l.Close()

	// Now checkout the archive.
	repoArchive := x.opts.RepoArchiveRegistry.Checkout(
		repozip.RepoRevision{
			RepoName: task.Repository.Name,
			Commit:   task.Repository.Rev(),
		},
		task.ArchivePathToFetch(),
	)

	// Actually execute the steps.
	opts := &RunStepsOpts{
		Task:             task,
		Logger:           l,
		WC:               x.opts.Creator,
		EnsureImage:      x.opts.EnsureImage,
		TempDir:          x.opts.TempDir,
		GlobalEnv:        x.opts.GlobalEnv,
		Timeout:          x.opts.Timeout,
		RepoArchive:      repoArchive,
		WorkingDirectory: x.opts.WorkingDirectory,
		ForceRoot:        x.opts.ForceRoot,
		BinaryDiffs:      x.opts.BinaryDiffs,

		UI: ui.StepsExecutionUI(task),
	}
	stepResults, err := RunSteps(ctx, opts)
	if err != nil {
		// Create a more visual error for the UI.
		err = TaskExecutionErr{
			Err:        err,
			Logfile:    l.Path(),
			Repository: task.Repository.Name,
		}
		l.MarkErrored()
	}

	return &taskResult{
		task:        task,
		stepResults: stepResults,
		err:         err,
	}, err
}

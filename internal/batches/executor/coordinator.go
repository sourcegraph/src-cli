package executor

import (
	"context"
	"reflect"
	"strconv"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/batches"
	"github.com/sourcegraph/src-cli/internal/batches/graphql"
	"github.com/sourcegraph/src-cli/internal/batches/workspace"
)

// Coordinates coordinates the execution of Tasks. It makes use of an Executor,
// checks the ExecutionCache whether execution is necessary, builds
// batches.ChangesetSpecs out of the executionResults.
type Coordinator struct {
	opts NewCoordinatorOpts

	cache ExecutionCache
}

type repoNameResolver func(ctx context.Context, name string) (*graphql.Repository, error)

type NewCoordinatorOpts struct {
	// Dependencies
	ResolveRepoName repoNameResolver
	Creator         workspace.Creator
	Client          api.Client

	// Everything that follows are either command-line flags or features.

	// TODO: We could probably have a wrapper around flags and features,
	// something like ExecutionArgs, that we can pass around
	CacheDir   string
	ClearCache bool
	SkipErrors bool

	// Used by createChangesetSpecs
	AutoAuthorDetails bool

	CleanArchives bool
	Parallelism   int
	Timeout       time.Duration
	KeepLogs      bool
	TempDir       string
}

func NewCoordinator(opts NewCoordinatorOpts) *Coordinator {
	return &Coordinator{
		opts: opts,

		cache: NewCache(opts.CacheDir),
	}
}

// CheckCache checks whether the internal ExecutionCache contains
// ChangesetSpecs for the given Tasks. If cached ChangesetSpecs exist, those
// are returned, otherwise the Task, to be executed later.
func (c *Coordinator) CheckCache(ctx context.Context, tasks []*Task) (uncached []*Task, specs []*batches.ChangesetSpec, err error) {
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

func (c *Coordinator) checkCacheForTask(ctx context.Context, task *Task) (specs []*batches.ChangesetSpec, found bool, err error) {
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

	specs, err = createChangesetSpecs(task, result, c.opts.AutoAuthorDetails)
	if err != nil {
		return specs, false, err
	}

	return specs, true, nil
}

type executionProgressPrinter func([]*TaskStatus)

func (c *Coordinator) ExecuteTasks(ctx context.Context, tasks []*Task, spec *batches.BatchSpec, printer executionProgressPrinter) ([]*batches.ChangesetSpec, []string, error) {
	var errs *multierror.Error

	// Start the goroutine that updates the UI
	status := NewTaskStatusCollection(tasks)

	done := make(chan struct{})
	if printer != nil {
		go func() {
			status.CopyStatuses(printer)

			ticker := time.NewTicker(1 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					status.CopyStatuses(printer)

				case <-done:
					return
				}
			}
		}()
	}

	// Setup executor

	exec := New(NewExecutorOpts{
		Status: status,

		Cache: c.cache,

		Client:  c.opts.Client,
		Creator: c.opts.Creator,

		CleanArchives:     c.opts.CleanArchives,
		AutoAuthorDetails: c.opts.AutoAuthorDetails,
		CacheDir:          c.opts.CacheDir,
		Parallelism:       c.opts.Parallelism,
		Timeout:           c.opts.Timeout,
		KeepLogs:          c.opts.KeepLogs,
		TempDir:           c.opts.TempDir,
	})

	// Run executor
	exec.Start(ctx, tasks)
	specs, err := exec.Wait(ctx)
	if printer != nil {
		status.CopyStatuses(printer)
		done <- struct{}{}
	}
	if err != nil {
		if c.opts.SkipErrors {
			errs = multierror.Append(errs, err)
		} else {
			return nil, nil, err
		}
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

			specs = append(specs, &batches.ChangesetSpec{
				BaseRepository:    repo.ID,
				ExternalChangeset: &batches.ExternalChangeset{ExternalID: sid},
			})
		}
	}

	return specs, exec.LogFiles(), errs.ErrorOrNil()
}

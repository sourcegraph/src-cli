package executor

import (
	"sync"
	"time"

	"github.com/sourcegraph/go-diff/diff"
	"github.com/sourcegraph/src-cli/internal/batches"
)

func NewStatusHubThing() *TaskStatusHubThing {
	return &TaskStatusHubThing{
		statuses: make(map[*Task]*TaskStatus),
	}
}

type TaskStatusHubThing struct {
	statuses   map[*Task]*TaskStatus
	statusesMu sync.RWMutex
}

func (hub *TaskStatusHubThing) AddTasks(tasks []*Task) {
	hub.statusesMu.Lock()
	defer hub.statusesMu.Unlock()

	for _, t := range tasks {
		hub.statuses[t] = &TaskStatus{
			RepoName:   t.Repository.Name,
			Path:       t.Path,
			EnqueuedAt: time.Now(),
		}
	}
}

func (hub *TaskStatusHubThing) UpdateTaskStatus(task *Task, update func(status *TaskStatus)) {
	hub.statusesMu.Lock()
	defer hub.statusesMu.Unlock()

	status, ok := hub.statuses[task]
	if ok {
		update(status)
	}
}

func (hub *TaskStatusHubThing) LockedTaskStatuses(callback func([]*TaskStatus)) {
	hub.statusesMu.RLock()
	defer hub.statusesMu.RUnlock()

	var s []*TaskStatus
	for _, status := range hub.statuses {
		s = append(s, status)
	}

	callback(s)
}

type TaskStatus struct {
	RepoName string
	Path     string

	Cached bool

	LogFile    string
	EnqueuedAt time.Time
	StartedAt  time.Time
	FinishedAt time.Time

	// TODO: add current step and progress fields.
	CurrentlyExecuting string

	// ChangesetSpecs are the specs produced by executing the Task in a
	// repository. With the introduction of `transformChanges` to the batch
	// spec, one Task can produce multiple ChangesetSpecs.
	ChangesetSpecs []*batches.ChangesetSpec
	// Err is set if executing the Task lead to an error.
	Err error

	fileDiffs     []*diff.FileDiff
	fileDiffsErr  error
	fileDiffsOnce sync.Once
}

func (ts *TaskStatus) DisplayName() string {
	if ts.Path != "" {
		return ts.RepoName + ":" + ts.Path
	}
	return ts.RepoName
}

func (ts *TaskStatus) IsRunning() bool {
	return !ts.StartedAt.IsZero() && ts.FinishedAt.IsZero()
}

func (ts *TaskStatus) IsCompleted() bool {
	return !ts.StartedAt.IsZero() && !ts.FinishedAt.IsZero()
}

func (ts *TaskStatus) ExecutionTime() time.Duration {
	return ts.FinishedAt.Sub(ts.StartedAt).Truncate(time.Millisecond)
}

// FileDiffs returns the file diffs produced by the Task in the given
// repository.
// If no file diffs were produced, the task resulted in an error, or the task
// hasn't finished execution yet, the second return value is false.
func (ts *TaskStatus) FileDiffs() ([]*diff.FileDiff, bool, error) {
	if !ts.IsCompleted() || len(ts.ChangesetSpecs) == 0 || ts.Err != nil {
		return nil, false, nil
	}

	ts.fileDiffsOnce.Do(func() {
		var all []*diff.FileDiff

		for _, spec := range ts.ChangesetSpecs {
			fd, err := diff.ParseMultiFileDiff([]byte(spec.Commits[0].Diff))
			if err != nil {
				ts.fileDiffsErr = err
				return
			}

			all = append(all, fd...)
		}

		ts.fileDiffs = all
	})

	return ts.fileDiffs, len(ts.fileDiffs) != 0, ts.fileDiffsErr
}

package executor

import (
	"github.com/sourcegraph/src-cli/internal/batches"
	"github.com/sourcegraph/src-cli/internal/batches/graphql"
)

type Task struct {
	Repository *graphql.Repository

	// Path is the folder relative to the repository's root in which the steps
	// should be executed.
	Path string
	// OnlyFetchWorkspace determines whether the repository archive contains
	// the complete repository or just the files in Path (and additional files,
	// see RepoFetcher).
	// If Path is "" then this setting has no effect.
	OnlyFetchWorkspace bool

	Steps []batches.Step

	// TODO(mrnugget): this should just be a single BatchSpec field instead, if
	// we can make it work with caching
	BatchChangeAttributes *BatchChangeAttributes     `json:"-"`
	Template              *batches.ChangesetTemplate `json:"-"`
	TransformChanges      *batches.TransformChanges  `json:"-"`

	Archive batches.RepoZip `json:"-"`

	// ----------------------------------------------------------------------------
	// EXPERIMENT STARTS HERE
	// vvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvv
	CachedResultFound bool             `json:"-"`
	CachedResult      cachedStepResult `json:"-"`
}

func (t *Task) ArchivePathToFetch() string {
	if t.OnlyFetchWorkspace {
		return t.Path
	}
	return ""
}

func (t *Task) cacheKey() ExecutionCacheKey {
	return ExecutionCacheKey{t}
}

// TODO: This is hacky, because we only do this to get an ExecutionCacheKey
func (t *Task) cacheKeyForSteps(i int) ExecutionCacheKey {
	taskCopy := &Task{
		Repository:            t.Repository,
		Path:                  t.Path,
		OnlyFetchWorkspace:    t.OnlyFetchWorkspace,
		BatchChangeAttributes: t.BatchChangeAttributes,
		Template:              t.Template,
		TransformChanges:      t.TransformChanges,
		Archive:               t.Archive,
	}

	taskCopy.Steps = t.Steps[0 : i+1]

	return ExecutionCacheKey{taskCopy}
}

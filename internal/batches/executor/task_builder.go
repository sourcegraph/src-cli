package executor

import (
	"github.com/gobwas/glob"
	"github.com/sourcegraph/src-cli/internal/batches"
	"github.com/sourcegraph/src-cli/internal/batches/graphql"
)

type TaskBuilder struct {
	spec             *batches.BatchSpec
	initializedSteps []batches.Step
}

func NewTaskBuilder(spec *batches.BatchSpec) (*TaskBuilder, error) {
	s := &TaskBuilder{spec: spec}

	for _, step := range spec.Steps {
		if step.In != "" {
			g, err := glob.Compile(step.In)
			if err != nil {
				return nil, err
			}
			step.SetInGlob(g)
		}

		s.initializedSteps = append(s.initializedSteps, step)
	}

	return s, nil
}

func (s *TaskBuilder) Build(r *graphql.Repository, path string, onlyWorkspace bool) *Task {
	var taskSteps []batches.Step
	for _, s := range s.initializedSteps {
		if s.InMatches(r.Name) {
			taskSteps = append(taskSteps, s)
		}
	}

	// "." means the path is root, but in the executor we use "" to signify root
	if path == "." {
		path = ""
	}

	return &Task{
		Repository:         r,
		Path:               path,
		Steps:              taskSteps,
		OnlyFetchWorkspace: onlyWorkspace,

		TransformChanges: s.spec.TransformChanges,
		Template:         s.spec.ChangesetTemplate,
		BatchChangeAttributes: &BatchChangeAttributes{
			Name:        s.spec.Name,
			Description: s.spec.Description,
		},
	}
}

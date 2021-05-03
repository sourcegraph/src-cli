package executor

import (
	"context"
	"fmt"

	"github.com/gobwas/glob"
	"github.com/sourcegraph/src-cli/internal/batches"
	"github.com/sourcegraph/src-cli/internal/batches/graphql"
)

type DirectoryFinder interface {
	FindDirectoriesInRepos(ctx context.Context, fileName string, repos ...*graphql.Repository) (map[*graphql.Repository][]string, error)
}

type TaskBuilder struct {
	spec   *batches.BatchSpec
	finder DirectoryFinder

	initializedSteps            []batches.Step
	initializedWorkspaceConfigs []batches.WorkspaceConfiguration
}

func NewTaskBuilder(spec *batches.BatchSpec, finder DirectoryFinder) (*TaskBuilder, error) {
	s := &TaskBuilder{spec: spec, finder: finder}

	for _, step := range spec.Steps {
		// TODO
		s.initializedSteps = append(s.initializedSteps, step)
	}

	for _, conf := range s.spec.Workspaces {
		g, err := glob.Compile(conf.In)
		if err != nil {
			return nil, err
		}
		conf.SetGlob(g)
		s.initializedWorkspaceConfigs = append(s.initializedWorkspaceConfigs, conf)
	}

	return s, nil
}

func (s *TaskBuilder) buildTask(r *graphql.Repository, path string, onlyWorkspace bool) *Task {
	var taskSteps []batches.Step
	for _, s := range s.initializedSteps {
		// TODO
		taskSteps = append(taskSteps, s)
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

func (s *TaskBuilder) BuildAll(ctx context.Context, repos []*graphql.Repository) ([]*Task, error) {
	// Find workspaces in repositories, if configured
	workspaces, root, err := s.findWorkspaces(ctx, repos, s.initializedWorkspaceConfigs)
	if err != nil {
		return nil, err
	}

	var tasks []*Task
	for repo, ws := range workspaces {
		for _, path := range ws.paths {
			t := s.buildTask(repo, path, ws.onlyFetchWorkspace)
			tasks = append(tasks, t)
		}
	}

	for _, repo := range root {
		tasks = append(tasks, s.buildTask(repo, "", false))
	}

	return tasks, nil
}

type repoWorkspaces struct {
	paths              []string
	onlyFetchWorkspace bool
}

// findWorkspaces matches the given repos to the workspace configs and
// searches, via the Sourcegraph instance, the locations of the workspaces in
// each repository.
// The repositories that were matched by a workspace config are returned in
// workspaces. root contains the repositories that didn't match a config.
// If the user didn't specify any workspaces, the repositories are returned as
// root repositories.
func (s *TaskBuilder) findWorkspaces(
	ctx context.Context,
	repos []*graphql.Repository,
	configs []batches.WorkspaceConfiguration,
) (workspaces map[*graphql.Repository]repoWorkspaces, root []*graphql.Repository, err error) {
	if len(configs) == 0 {
		return nil, repos, nil
	}

	matched := map[int][]*graphql.Repository{}

	for _, repo := range repos {
		found := false

		for idx, conf := range configs {
			if !conf.Matches(repo.Name) {
				continue
			}

			if found {
				return nil, nil, fmt.Errorf("repository %s matches multiple workspaces.in globs in the batch spec. glob: %q", repo.Name, conf.In)
			}

			matched[idx] = append(matched[idx], repo)
			found = true
		}

		if !found {
			root = append(root, repo)
		}
	}

	workspaces = map[*graphql.Repository]repoWorkspaces{}
	for idx, repos := range matched {
		conf := configs[idx]
		repoDirs, err := s.finder.FindDirectoriesInRepos(ctx, conf.RootAtLocationOf, repos...)
		if err != nil {
			return nil, nil, err
		}

		for repo, dirs := range repoDirs {
			workspaces[repo] = repoWorkspaces{
				paths:              dirs,
				onlyFetchWorkspace: conf.OnlyFetchWorkspace,
			}
		}
	}

	return workspaces, root, nil
}

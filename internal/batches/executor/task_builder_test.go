package executor

import (
	"context"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/sourcegraph/src-cli/internal/batches"
	"github.com/sourcegraph/src-cli/internal/batches/graphql"
)

func TestTaskBuilder_Build_Globbing(t *testing.T) {
	repo := &graphql.Repository{Name: "github.com/sourcegraph/automation-testing"}

	tests := map[string]struct {
		spec *batches.BatchSpec

		wantSteps []batches.Step
	}{
		"no globbing": {
			spec: &batches.BatchSpec{
				Steps: []batches.Step{
					{Run: "echo 1"},
				},
			},
			wantSteps: []batches.Step{
				{Run: "echo 1"},
			},
		},

		"glob matches": {
			spec: &batches.BatchSpec{
				Steps: []batches.Step{
					{Run: "echo 1", In: "github.com*"},
				},
			},
			wantSteps: []batches.Step{
				{Run: "echo 1", In: "github.com*"},
			},
		},

		"glob does not match": {
			spec: &batches.BatchSpec{
				Steps: []batches.Step{
					{Run: "echo 1", In: "bitbucket"},
				},
			},
			wantSteps: nil,
		},

		"glob matches subset of steps": {
			spec: &batches.BatchSpec{
				Steps: []batches.Step{
					{Run: "echo 1", In: "github.com*"},
					{Run: "echo 2"},
					{Run: "echo 3", In: "bitbucket"},
					{Run: "echo 4", In: "bitbucket"},
					{Run: "echo 5", In: "*automation-testing*"},
				},
			},
			wantSteps: []batches.Step{
				{Run: "echo 1", In: "github.com*"},
				{Run: "echo 2"},
				{Run: "echo 5", In: "*automation-testing*"},
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			builder, err := NewTaskBuilder(tt.spec, nil)
			if err != nil {
				t.Fatal(err)
			}
			task := builder.Build(repo, "", false)
			if err != nil {
				t.Fatalf("unexpected err: %s", err)
			}

			opts := cmpopts.IgnoreUnexported(batches.Step{})
			if diff := cmp.Diff(tt.wantSteps, task.Steps, opts); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestTaskBuilder_BuildAll_Workspaces(t *testing.T) {
	repos := []*graphql.Repository{
		{ID: "repo-id-0", Name: "github.com/sourcegraph/automation-testing"},
		{ID: "repo-id-1", Name: "github.com/sourcegraph/sourcegraph"},
		{ID: "repo-id-2", Name: "bitbucket.sgdev.org/SOUR/automation-testing"},
	}

	type finderResults map[*graphql.Repository][]string

	type wantTask struct {
		Path               string
		ArchivePathToFetch string
	}

	tests := map[string]struct {
		spec          *batches.BatchSpec
		finderResults map[*graphql.Repository][]string

		wantNumTasks int

		// tasks per repository ID and in which path they are executed
		wantTasks map[string][]wantTask
	}{
		"no workspace configuration": {
			spec:          &batches.BatchSpec{},
			finderResults: finderResults{},
			wantNumTasks:  len(repos),
			wantTasks: map[string][]wantTask{
				repos[0].ID: {{Path: ""}},
				repos[1].ID: {{Path: ""}},
				repos[2].ID: {{Path: ""}},
			},
		},

		"workspace configuration matching no repos": {
			spec: &batches.BatchSpec{
				Workspaces: []batches.WorkspaceConfiguration{
					{In: "this-does-not-match", RootAtLocationOf: "package.json"},
				},
			},
			finderResults: finderResults{},
			wantNumTasks:  len(repos),
			wantTasks: map[string][]wantTask{
				repos[0].ID: {{Path: ""}},
				repos[1].ID: {{Path: ""}},
				repos[2].ID: {{Path: ""}},
			},
		},

		"workspace configuration matching 2 repos with no results": {
			spec: &batches.BatchSpec{
				Workspaces: []batches.WorkspaceConfiguration{
					{In: "*automation-testing", RootAtLocationOf: "package.json"},
				},
			},
			finderResults: finderResults{
				repos[0]: []string{},
				repos[2]: []string{},
			},
			wantNumTasks: 1,
			wantTasks: map[string][]wantTask{
				repos[1].ID: {{Path: ""}},
			},
		},

		"workspace configuration matching 2 repos with 3 results each": {
			spec: &batches.BatchSpec{
				Workspaces: []batches.WorkspaceConfiguration{
					{In: "*automation-testing", RootAtLocationOf: "package.json"},
				},
			},
			finderResults: finderResults{
				repos[0]: {"a/b", "a/b/c", "d/e/f"},
				repos[2]: {"a/b", "a/b/c", "d/e/f"},
			},
			wantNumTasks: 7,
			wantTasks: map[string][]wantTask{
				repos[0].ID: {{Path: "a/b"}, {Path: "a/b/c"}, {Path: "d/e/f"}},
				repos[1].ID: {{Path: ""}},
				repos[2].ID: {{Path: "a/b"}, {Path: "a/b/c"}, {Path: "d/e/f"}},
			},
		},

		"workspace configuration matches repo with OnlyFetchWorkspace": {
			spec: &batches.BatchSpec{
				Workspaces: []batches.WorkspaceConfiguration{
					{
						OnlyFetchWorkspace: true,
						In:                 "*automation-testing",
						RootAtLocationOf:   "package.json",
					},
				},
			},
			finderResults: finderResults{
				repos[0]: {"a/b", "a/b/c", "d/e/f"},
				repos[2]: {"a/b", "a/b/c", "d/e/f"},
			},
			wantNumTasks: 7,
			wantTasks: map[string][]wantTask{
				repos[0].ID: {
					{Path: "a/b", ArchivePathToFetch: "a/b"},
					{Path: "a/b/c", ArchivePathToFetch: "a/b/c"},
					{Path: "d/e/f", ArchivePathToFetch: "d/e/f"},
				},
				repos[1].ID: {{Path: ""}},
				repos[2].ID: {
					{Path: "a/b", ArchivePathToFetch: "a/b"},
					{Path: "a/b/c", ArchivePathToFetch: "a/b/c"},
					{Path: "d/e/f", ArchivePathToFetch: "d/e/f"},
				},
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			finder := &mockDirectoryFinder{results: tt.finderResults}
			tb, err := NewTaskBuilder(tt.spec, finder)
			if err != nil {
				t.Fatal(err)
			}

			tasks, err := tb.BuildAll(context.Background(), repos)
			if err != nil {
				t.Fatalf("unexpected err: %s", err)
			}

			if have := len(tasks); have != tt.wantNumTasks {
				t.Fatalf("wrong number of tasks. want=%d, got=%d", tt.wantNumTasks, have)
			}

			haveTasks := map[string][]wantTask{}
			for _, task := range tasks {
				haveTasks[task.Repository.ID] = append(haveTasks[task.Repository.ID], wantTask{
					Path:               task.Path,
					ArchivePathToFetch: task.ArchivePathToFetch(),
				})
			}

			for _, tasks := range haveTasks {
				sort.Slice(tasks, func(i, j int) bool { return tasks[i].Path < tasks[j].Path })
			}

			if diff := cmp.Diff(tt.wantTasks, haveTasks); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

type mockDirectoryFinder struct {
	results map[*graphql.Repository][]string
}

func (m *mockDirectoryFinder) FindDirectoriesInRepos(ctx context.Context, fileName string, repos ...*graphql.Repository) (map[*graphql.Repository][]string, error) {
	return m.results, nil
}

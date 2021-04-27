package executor

import (
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
			builder, err := NewTaskBuilder(tt.spec)
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

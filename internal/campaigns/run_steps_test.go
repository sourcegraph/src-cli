package campaigns

import (
	"bytes"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/sourcegraph/src-cli/internal/campaigns/graphql"
)

func TestParseGitStatus(t *testing.T) {
	const input = `M  README.md
M  another_file.go
A  new_file.txt
A  barfoo/new_file.txt
D  to_be_deleted.txt
`
	parsed, err := parseGitStatus([]byte(input))
	if err != nil {
		t.Fatal(err)
	}

	want := StepChanges{
		Modified: []string{"README.md", "another_file.go"},
		Added:    []string{"new_file.txt", "barfoo/new_file.txt"},
		Deleted:  []string{"to_be_deleted.txt"},
	}

	if !cmp.Equal(want, parsed) {
		t.Fatalf("wrong output:\n%s", cmp.Diff(want, parsed))
	}
}

func TestParseStepRun(t *testing.T) {
	tests := []struct {
		stepCtx StepContext
		run     string
		want    string
	}{
		{
			stepCtx: StepContext{
				Repository: &graphql.Repository{Name: "github.com/sourcegraph/src-cli"},
				PreviousStep: StepResult{
					Changes: StepChanges{
						Modified: []string{"go.mod"},
						Added:    []string{"main.go.swp"},
						Deleted:  []string{".DS_Store"},
					},
				},
			},

			run:  `${{ .PreviousStep.ModifiedFiles }} ${{ .Repository.Name }}`,
			want: `go.mod github.com/sourcegraph/src-cli`,
		},
		{
			stepCtx: StepContext{
				Repository: &graphql.Repository{Name: "github.com/sourcegraph/src-cli"},
			},

			run:  `${{ .PreviousStep.ModifiedFiles }} ${{ .Repository.Name }}`,
			want: ` github.com/sourcegraph/src-cli`,
		},
		{
			stepCtx: StepContext{
				Repository: &graphql.Repository{
					Name: "github.com/sourcegraph/src-cli",
					FileMatches: map[string]bool{
						"README.md": true,
						"main.go":   true,
					},
				},
			},

			run:  `${{ .Repository.SearchResultPaths }}`,
			want: `README.md main.go`,
		},
	}

	for _, tc := range tests {
		parsed, err := parseStepRun(tc.run)
		if err != nil {
			t.Fatal(err)
		}

		var out bytes.Buffer
		if err := parsed.Execute(&out, tc.stepCtx); err != nil {
			t.Fatalf("executing template failed: %s", err)
		}

		if out.String() != tc.want {
			t.Fatalf("wrong output:\n%s", cmp.Diff(tc.want, out.String()))
		}
	}
}

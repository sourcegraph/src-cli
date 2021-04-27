package executor

import (
	"bytes"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/sourcegraph/src-cli/internal/batches/git"
	"github.com/sourcegraph/src-cli/internal/batches/graphql"
)

// func TestGoVariadicBuiltins(t *testing.T) {
// 	input := `${{ coolFunc "-" (split "-" "a-b-c-d-e-f") }}`
// 	tmp, err := template.
// 		New("partial-eval").
// 		Delims(startDelim, endDelim).
// 		Funcs(builtins).
// 		Parse(input)
//
// 	if err != nil {
// 		t.Fatal(err)
// 	}
//
// 	var out bytes.Buffer
// 	err = tmp.Execute(&out, nil)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
//
// 	if out.String() != "abcde" {
// 		t.Fatalf("wrong output: %q", out.String())
// 	}
// }
//
var partialEvalStepCtx = &StepContext{
	BatchChange: BatchChangeAttributes{
		Name:        "test-batch-change",
		Description: "test-description",
	},
	PreviousStep: StepResult{
		files: &git.Changes{
			Modified: []string{"go.mod"},
			Added:    []string{"main.go.swp"},
			Deleted:  []string{".DS_Store"},
			Renamed:  []string{"new-filename.txt"},
		},
		Stdout: bytes.NewBufferString("this is previous step's stdout"),
		Stderr: bytes.NewBufferString("this is previous step's stderr"),
	},
	Outputs: map[string]interface{}{
		"output1": "output-value-1",
	},
	// Step is not set when evalStepCondition is called
	Repository: graphql.Repository{
		Name: "github.com/sourcegraph/src-cli",
		FileMatches: map[string]bool{
			"README.md": true,
			"main.go":   true,
		},
	},
}

func runParseAndPartialTest(t *testing.T, in, want string) {
	t.Helper()

	tmpl, err := parseAndPartialEval(in, partialEvalStepCtx)
	if err != nil {
		t.Fatal(err)
	}

	tmplStr := tmpl.Tree.Root.String()
	if tmplStr != want {
		t.Fatalf("wrong output:\n%s", cmp.Diff(want, tmplStr))
	}
}

func TestParseAndPartialEval(t *testing.T) {
	t.Run("evaluated", func(t *testing.T) {
		for _, tt := range []struct{ input, want string }{
			{
				// Literal constants:
				`this is my template ${{ "hardcoded string" }}`,
				`this is my template hardcoded string`,
			},
			{
				`${{ 1234 }}`,
				`1234`,
			},
			{
				`${{ true }} ${{ false }}`,
				`true false`,
			},
			{
				// Evaluated, since they're static values:
				`${{ repository.name }} ${{ batch_change.name }} ${{ batch_change.description }}`,
				`github.com/sourcegraph/src-cli test-batch-change test-description`,
			},
			{
				`AAA${{ repository.name }}BBB${{ batch_change.name }}CCC${{ batch_change.description }}DDD`,
				`AAAgithub.com/sourcegraph/src-cliBBBtest-batch-changeCCCtest-descriptionDDD`,
			},
			{
				// Function call with static value and runtime value:
				`${{ eq repository.name outputs.repo.name }}`,
				// Aborts, since one of them is runtime value
				`{{eq repository.name outputs.repo.name}}`,
			},
			{
				// "eq" call with 2 static values:
				`${{ eq repository.name "github.com/sourcegraph/src-cli" }}`,
				`true`,
			},
			{
				// "eq" call with 2 literal values:
				`${{ eq 5 5 }}`,
				`true`,
			},
			{
				// Function call with builtin function and static values:
				`${{ matches repository.name "github.com*" }}`,
				`true`,
			},
			{
				// Nested function call with builtin function and static values:
				`${{ eq false (matches repository.name "github.com*") }}`,
				`false`,
			},
			{
				// Nested nested function call with builtin function and static values:
				`${{ eq false (eq false (matches repository.name "github.com*")) }}`,
				`true`,
			},
		} {
			runParseAndPartialTest(t, tt.input, tt.want)
		}
	})

	t.Run("aborted", func(t *testing.T) {
		for _, tt := range []struct{ input, want string }{
			{
				// Complex value
				`${{ repository.search_result_paths }}`,
				// String representation of templates uses standard delimiters
				`{{repository.search_result_paths}}`,
			},
			{
				// Runtime value
				`${{ outputs.runtime.value }}`,
				`{{outputs.runtime.value}}`,
			},
			{
				// Runtime value
				`${{ step.modified_files }}`,
				`{{step.modified_files}}`,
			},
			{
				// "eq" call with static value and runtime value:
				`${{ eq repository.name outputs.repo.name }}`,
				// Aborts, since one of them is runtime value
				`{{eq repository.name outputs.repo.name}}`,
			},
			{
				// "eq" call with more than 2 arguments:
				`${{ eq repository.name "github.com/sourcegraph/src-cli" "github.com/sourcegraph/sourcegraph" }}`,
				`{{eq repository.name "github.com/sourcegraph/src-cli" "github.com/sourcegraph/sourcegraph"}}`,
			},
			{
				// Nested nested function call with builtin function but runtime values:
				`${{ eq false (eq false (matches outputs.runtime.value "github.com*")) }}`,
				`{{eq false (eq false (matches outputs.runtime.value "github.com*"))}}`,
			},
		} {
			runParseAndPartialTest(t, tt.input, tt.want)
		}
	})
}

func TestParseAndPartialEval_BuiltinFunctions(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		for _, tt := range []struct{ input, want string }{
			{
				`${{ join (split repository.name "/") "-" }}`,
				`github.com-sourcegraph-src-cli`,
			},
			{
				`${{ split repository.name "/" "-" }}`,
				`{{split repository.name "/" "-"}}`,
			},
			{
				`${{ replace repository.name "github" "foobar" }}`,
				`foobar.com/sourcegraph/src-cli`,
			},
			{
				`${{ join_if "SEP" repository.name "postfix" }}`,
				`github.com/sourcegraph/src-cliSEPpostfix`,
			},
			{
				`${{ matches repository.name "github.com*" }}`,
				`true`,
			},
		} {
			runParseAndPartialTest(t, tt.input, tt.want)
		}
	})

	t.Run("aborted", func(t *testing.T) {
		for _, tt := range []struct{ input, want string }{
			{
				// Wrong argument type
				`${{ join "foobar" "-" }}`,
				`{{join "foobar" "-"}}`,
			},
			{
				// Wrong argument count
				`${{ join (split repository.name "/") "-" "too" "many" "args" }}`,
				`{{join (split repository.name "/") "-" "too" "many" "args"}}`,
			},
		} {
			runParseAndPartialTest(t, tt.input, tt.want)
		}
	})
}

func TestIsStaticBool(t *testing.T) {
	tests := []struct {
		name         string
		template     string
		wantIsStatic bool
		wantBoolVal  bool
	}{

		{
			name:         "true literal",
			template:     `true`,
			wantIsStatic: true,
			wantBoolVal:  true,
		},
		{
			name:         "false literal",
			template:     `false`,
			wantIsStatic: true,
			wantBoolVal:  false,
		},
		{
			name:         "static non bool value",
			template:     `${{ repository.name }}`,
			wantIsStatic: true,
			wantBoolVal:  false,
		},
		{
			name:         "function call true val",
			template:     `${{ eq repository.name "github.com/sourcegraph/src-cli" }}`,
			wantIsStatic: true,
			wantBoolVal:  true,
		},
		{
			name:         "function call false val",
			template:     `${{ eq repository.name "hans wurst" }}`,
			wantIsStatic: true,
			wantBoolVal:  false,
		},
		{
			name:         "nested function call and whitespace",
			template:     `   ${{ eq false (eq false (matches repository.name "github.com*")) }}   `,
			wantIsStatic: true,
			wantBoolVal:  true,
		},
		{
			name:         "nested function call with runtime value",
			template:     `${{ eq false (eq false (matches outputs.repo.name "github.com*")) }}`,
			wantIsStatic: false,
			wantBoolVal:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			isStatic, boolVal, err := isStaticBool(tt.template, partialEvalStepCtx)
			if err != nil {
				t.Fatal(err)
			}

			if isStatic != tt.wantIsStatic {
				t.Fatalf("wrong isStatic value. want=%t, got=%t", tt.wantIsStatic, isStatic)
			}
			if boolVal != tt.wantBoolVal {
				t.Fatalf("wrong boolVal value. want=%t, got=%t", tt.wantBoolVal, boolVal)
			}
		})
	}
}

package executor

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"github.com/sourcegraph/go-diff/diff"
	batcheslib "github.com/sourcegraph/sourcegraph/lib/batches"
	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/batches"
	"github.com/sourcegraph/src-cli/internal/batches/docker"
	"github.com/sourcegraph/src-cli/internal/batches/git"
	"github.com/sourcegraph/src-cli/internal/batches/mock"
	"github.com/sourcegraph/src-cli/internal/batches/template"
	"github.com/sourcegraph/src-cli/internal/batches/workspace"
)

func TestExecutor_Integration(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Test doesn't work on Windows because dummydocker is written in bash")
	}

	addToPath(t, "testdata/dummydocker")

	defaultBatchChangeAttributes := &template.BatchChangeAttributes{
		Name:        "integration-test-batch-change",
		Description: "this is an integration test",
	}

	const rootPath = ""
	type filesByPath map[string][]string
	type filesByRepository map[string]filesByPath

	tests := []struct {
		name string

		archives        []mock.RepoArchive
		additionalFiles []mock.MockRepoAdditionalFiles

		// We define the steps only once per test case so there's less duplication
		steps []batcheslib.Step
		tasks []*Task

		executorTimeout time.Duration

		wantFilesChanged  filesByRepository
		wantTitle         string
		wantBody          string
		wantCommitMessage string
		wantAuthorName    string
		wantAuthorEmail   string

		wantErrInclude string

		wantFinished        int
		wantFinishedWithErr int
	}{
		{
			name: "success",
			archives: []mock.RepoArchive{
				{Repo: testRepo1, Files: map[string]string{
					"README.md": "# Welcome to the README\n",
					"main.go":   "package main\n\nfunc main() {\n\tfmt.Println(     \"Hello World\")\n}\n",
				}},
				{Repo: testRepo2, Files: map[string]string{
					"README.md": "# Sourcegraph README\n",
				}},
			},
			steps: []batcheslib.Step{
				{Run: `echo -e "foobar\n" >> README.md`},
				{Run: `[[ -f "main.go" ]] && go fmt main.go || exit 0`},
			},
			tasks: []*Task{
				{Repository: testRepo1},
				{Repository: testRepo2},
			},
			wantFilesChanged: filesByRepository{
				testRepo1.ID: filesByPath{
					rootPath: []string{"README.md", "main.go"},
				},
				testRepo2.ID: {
					rootPath: []string{"README.md"},
				},
			},
			wantFinished: 2,
		},
		{
			name: "empty",
			archives: []mock.RepoArchive{
				{Repo: testRepo1, Files: map[string]string{
					"README.md": "# Welcome to the README\n",
					"main.go":   "package main\n\nfunc main() {\n\tfmt.Println(     \"Hello World\")\n}\n",
				}},
			},
			steps: []batcheslib.Step{
				{Run: "true"},
			},

			tasks: []*Task{
				{Repository: testRepo1},
			},
			// No diff should be generated.
			wantFilesChanged: filesByRepository{
				testRepo1.ID: filesByPath{
					rootPath: []string{},
				},
			},
			wantFinished: 1,
		},
		{
			name: "timeout",
			archives: []mock.RepoArchive{
				{Repo: testRepo1, Files: map[string]string{"README.md": "line 1"}},
			},
			steps: []batcheslib.Step{
				// This needs to be a loop, because when a process goes to sleep
				// it's not interruptible, meaning that while it will receive SIGKILL
				// it won't exit until it had its full night of sleep.
				// So.
				// Instead we take short powernaps.
				{Run: `while true; do echo "zZzzZ" && sleep 0.05; done`},
			},
			tasks: []*Task{
				{Repository: testRepo1},
			},
			executorTimeout:     100 * time.Millisecond,
			wantErrInclude:      "execution in github.com/sourcegraph/src-cli failed: Timeout reached. Execution took longer than 100ms.",
			wantFinishedWithErr: 1,
		},
		{
			name: "templated steps",
			archives: []mock.RepoArchive{
				{Repo: testRepo1, Files: map[string]string{
					"README.md": "# Welcome to the README\n",
					"main.go":   "package main\n\nfunc main() {\n\tfmt.Println(     \"Hello World\")\n}\n",
				}},
			},
			steps: []batcheslib.Step{
				{Run: `go fmt main.go`},
				{Run: `touch modified-${{ join previous_step.modified_files " " }}.md`},
				{Run: `touch added-${{ join previous_step.added_files " " }}`},
				{
					Run: `echo "hello.txt"`,
					Outputs: batcheslib.Outputs{
						"myOutput": batcheslib.Output{
							Value: "${{ step.stdout }}",
						},
					},
				},
				{Run: `touch output-${{ outputs.myOutput }}`},
			},

			tasks: []*Task{
				{Repository: testRepo1},
			},
			wantFilesChanged: filesByRepository{
				testRepo1.ID: filesByPath{
					rootPath: []string{
						"main.go",
						"modified-main.go.md",
						"added-modified-main.go.md",
						"output-hello.txt",
					},
				},
			},
			wantFinished: 1,
		},
		{
			name: "workspaces",
			archives: []mock.RepoArchive{
				{Repo: testRepo1, Path: "", Files: map[string]string{
					".gitignore":      "node_modules",
					"message.txt":     "root-dir",
					"a/message.txt":   "a-dir",
					"a/.gitignore":    "node_modules-in-a",
					"a/b/message.txt": "b-dir",
				}},
				{Repo: testRepo1, Path: "a", Files: map[string]string{
					"a/message.txt":   "a-dir",
					"a/.gitignore":    "node_modules-in-a",
					"a/b/message.txt": "b-dir",
				}},
				{Repo: testRepo1, Path: "a/b", Files: map[string]string{
					"a/b/message.txt": "b-dir",
				}},
			},
			additionalFiles: []mock.MockRepoAdditionalFiles{
				{Repo: testRepo1, AdditionalFiles: map[string]string{
					".gitignore":   "node_modules",
					"a/.gitignore": "node_modules-in-a",
				}},
			},
			steps: []batcheslib.Step{
				{
					Run: "cat message.txt && echo 'Hello' > hello.txt",
					Outputs: batcheslib.Outputs{
						"message": batcheslib.Output{
							Value: "${{ step.stdout }}",
						},
					},
				},
				{Run: `if [[ -f ".gitignore" ]]; then echo "yes" >> gitignore-exists; fi`},
				{Run: `if [[ $(basename $(pwd)) == "a" && -f "../.gitignore" ]]; then echo "yes" >> gitignore-exists; fi`},
				// In `a/b` we want the `.gitignore` file in the root folder and in `a` to be fetched:
				{Run: `if [[ $(basename $(pwd)) == "b" && -f "../../.gitignore" ]]; then echo "yes" >> gitignore-exists; fi`},
				{Run: `if [[ $(basename $(pwd)) == "b" && -f "../.gitignore" ]]; then echo "yes" >> gitignore-exists-in-a; fi`},
			},
			tasks: []*Task{
				{Repository: testRepo1, Path: ""},
				{Repository: testRepo1, Path: "a"},
				{Repository: testRepo1, Path: "a/b"},
			},

			wantFilesChanged: filesByRepository{
				testRepo1.ID: filesByPath{
					rootPath: []string{"hello.txt", "gitignore-exists"},
					"a":      []string{"a/hello.txt", "a/gitignore-exists"},
					"a/b":    []string{"a/b/hello.txt", "a/b/gitignore-exists", "a/b/gitignore-exists-in-a"},
				},
			},
			wantFinished: 3,
		},
		{
			name: "step condition",
			archives: []mock.RepoArchive{
				{Repo: testRepo1, Files: map[string]string{
					"README.md": "# Welcome to the README\n",
				}},
				{Repo: testRepo2, Files: map[string]string{
					"README.md": "# Sourcegraph README\n",
				}},
			},
			steps: []batcheslib.Step{
				{Run: `echo -e "foobar\n" >> README.md`},
				{
					Run: `echo "foobar" >> hello.txt`,
					If:  `${{ matches repository.name "github.com/sourcegraph/sourcegra*" }}`,
				},
				{
					Run: `echo "foobar" >> in-path.txt`,
					If:  `${{ matches steps.path "sub/directory/of/repo" }}`,
				},
			},
			tasks: []*Task{
				{Repository: testRepo1},
				{Repository: testRepo2, Path: "sub/directory/of/repo"},
			},
			wantFilesChanged: filesByRepository{
				testRepo1.ID: filesByPath{
					rootPath: []string{"README.md"},
				},
				testRepo2.ID: {
					"sub/directory/of/repo": []string{"README.md", "hello.txt", "in-path.txt"},
				},
			},
			wantFinished: 2,
		},
		{
			name: "skips errors",
			archives: []mock.RepoArchive{
				{Repo: testRepo1, Files: map[string]string{
					"README.md": "# Welcome to the README\n",
				}},
				{Repo: testRepo2, Files: map[string]string{
					"README.md": "# Sourcegraph README\n",
				}},
			},
			steps: []batcheslib.Step{
				{Run: `echo -e "foobar\n" >> README.md`},
				{
					Run: `exit 1`,
					If:  fmt.Sprintf(`${{ eq repository.name %q }}`, testRepo2.Name),
				},
			},
			tasks: []*Task{
				{Repository: testRepo1},
				{Repository: testRepo2},
			},
			wantFilesChanged: filesByRepository{
				testRepo1.ID: filesByPath{
					rootPath: []string{"README.md"},
				},
				testRepo2.ID: {},
			},
			wantErrInclude:      "execution in github.com/sourcegraph/sourcegraph failed: run: exit 1",
			wantFinished:        1,
			wantFinishedWithErr: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Make sure that the steps and tasks are setup properly
			images := make(map[string]docker.Image)
			for _, step := range tc.steps {
				images[step.Container] = &mock.Image{RawDigest: step.Container}
			}
			for _, task := range tc.tasks {
				task.BatchChangeAttributes = defaultBatchChangeAttributes
				task.Steps = tc.steps
			}

			// Setup a mock test server so we also test the downloading of archives
			mux := mock.NewZipArchivesMux(t, nil, tc.archives...)

			middle := func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					next.ServeHTTP(w, r)
				})
			}
			for _, additionalFiles := range tc.additionalFiles {
				mock.HandleAdditionalFiles(mux, additionalFiles, middle)
			}

			ts := httptest.NewServer(mux)
			defer ts.Close()

			// Setup an api.Client that points to this test server
			var clientBuffer bytes.Buffer
			client := api.NewClient(api.ClientOpts{Endpoint: ts.URL, Out: &clientBuffer})

			// Temp dir for log files and downloaded archives
			testTempDir, err := ioutil.TempDir("", "executor-integration-test-*")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(testTempDir)

			// Setup executor
			opts := newExecutorOpts{
				Creator:     workspace.NewCreator(context.Background(), "bind", testTempDir, testTempDir, images),
				Fetcher:     batches.NewRepoFetcher(client, testTempDir, false),
				Logger:      mock.LogNoOpManager{},
				EnsureImage: imageMapEnsurer(images),

				TempDir:     testTempDir,
				Parallelism: runtime.GOMAXPROCS(0),
				Timeout:     tc.executorTimeout,
			}

			if opts.Timeout == 0 {
				opts.Timeout = 30 * time.Second
			}

			dummyUI := newDummyTaskExecutionUI()
			executor := newExecutor(opts)

			// Run executor
			executor.Start(context.Background(), tc.tasks, dummyUI)

			results, err := executor.Wait(context.Background())
			if tc.wantErrInclude == "" {
				if err != nil {
					t.Fatalf("execution failed: %s", err)
				}
			} else {
				if err == nil {
					t.Fatalf("expected error to include %q, but got no error", tc.wantErrInclude)
				} else {
					if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.wantErrInclude)) {
						t.Errorf("wrong error. have=%q want included=%q", err, tc.wantErrInclude)
					}
				}
			}

			wantResults := 0
			resultsFound := map[string]map[string]bool{}
			for repo, byPath := range tc.wantFilesChanged {
				wantResults += len(byPath)
				resultsFound[repo] = map[string]bool{}
				for path := range byPath {
					resultsFound[repo][path] = false
				}
			}

			if have, want := len(results), wantResults; have != want {
				t.Fatalf("wrong number of execution results. want=%d, have=%d", want, have)
			}

			for _, taskResult := range results {
				repoID := taskResult.task.Repository.ID
				path := taskResult.task.Path

				wantFiles, ok := tc.wantFilesChanged[repoID]
				if !ok {
					t.Fatalf("unexpected file changes in repo %s", repoID)
				}

				resultsFound[repoID][path] = true

				wantFilesInPath, ok := wantFiles[path]
				if !ok {
					t.Fatalf("spec for repo %q and path %q but no files expected in that branch", repoID, path)
				}

				fileDiffs, err := diff.ParseMultiFileDiff([]byte(taskResult.result.Diff))
				if err != nil {
					t.Fatalf("failed to parse diff: %s", err)
				}

				if have, want := len(fileDiffs), len(wantFilesInPath); have != want {
					t.Fatalf("repo %s: wrong number of fileDiffs. want=%d, have=%d", repoID, want, have)
				}

				diffsByName := map[string]*diff.FileDiff{}
				for _, fd := range fileDiffs {
					if fd.NewName == "/dev/null" {
						diffsByName[fd.OrigName] = fd
					} else {
						diffsByName[fd.NewName] = fd
					}
				}

				for _, file := range wantFilesInPath {
					if _, ok := diffsByName[file]; !ok {
						t.Errorf("%s was not changed (diffsByName=%#v)", file, diffsByName)
					}
				}
			}

			for repo, paths := range resultsFound {
				for path, found := range paths {
					for !found {
						t.Fatalf("expected spec to be created in path %s of repo %s, but was not", path, repo)
					}
				}
			}

			// Make sure that all the Tasks have been updated correctly
			if have, want := len(dummyUI.finished), tc.wantFinished; have != want {
				t.Fatalf("wrong number of finished tasks. want=%d, have=%d", want, have)
			}
			if have, want := len(dummyUI.finishedWithErr), tc.wantFinishedWithErr; have != want {
				t.Fatalf("wrong number of finished-with-err tasks. want=%d, have=%d", want, have)
			}
		})
	}
}

func addToPath(t *testing.T, relPath string) {
	t.Helper()

	dummyDockerPath, err := filepath.Abs("testdata/dummydocker")
	if err != nil {
		t.Fatal(err)
	}
	os.Setenv("PATH", fmt.Sprintf("%s%c%s", dummyDockerPath, os.PathListSeparator, os.Getenv("PATH")))
}

func featuresAllEnabled() batches.FeatureFlags {
	return batches.FeatureFlags{
		AllowArrayEnvironments:   true,
		IncludeAutoAuthorDetails: true,
		UseGzipCompression:       true,
		AllowTransformChanges:    true,
		AllowWorkspaces:          true,
		AllowConditionalExec:     true,
		AllowOptionalPublished:   true,
	}
}

func TestExecutor_CachedStepResults(t *testing.T) {
	t.Run("single step cached", func(t *testing.T) {
		archive := mock.RepoArchive{
			Repo: testRepo1, Files: map[string]string{
				"README.md": "# Welcome to the README\n",
			},
		}

		cachedDiff := []byte(`diff --git README.md README.md
index 02a19af..c9644dd 100644
--- README.md
+++ README.md
@@ -1 +1,2 @@
 # Welcome to the README
+foobar
`)

		task := &Task{
			BatchChangeAttributes: &template.BatchChangeAttributes{},
			Steps: []batcheslib.Step{
				{Run: `echo -e "foobar\n" >> README.md`},
			},
			CachedResultFound: true,
			CachedResult: stepExecutionResult{
				StepIndex:          0,
				Diff:               cachedDiff,
				Outputs:            map[string]interface{}{},
				PreviousStepResult: template.StepResult{},
			},
			Repository: testRepo1,
		}

		results, err := testExecuteTasks(t, []*Task{task}, archive)
		if err != nil {
			t.Fatalf("execution failed: %s", err)
		}

		if have, want := len(results), 1; have != want {
			t.Fatalf("wrong number of execution results. want=%d, have=%d", want, have)
		}

		// We want the diff to be the same as the cached one, since we only had to
		// execute a single step
		executionResult := results[0].result
		if diff := cmp.Diff(executionResult.Diff, string(cachedDiff)); diff != "" {
			t.Fatalf("wrong diff: %s", diff)
		}

		if have, want := len(results[0].stepResults), 1; have != want {
			t.Fatalf("wrong length of step results. have=%d, want=%d", have, want)
		}

		stepResult := results[0].stepResults[0]
		if diff := cmp.Diff(stepResult, task.CachedResult); diff != "" {
			t.Fatalf("wrong stepResult: %s", diff)
		}
	})

	t.Run("one of multiple steps cached", func(t *testing.T) {
		archive := mock.RepoArchive{
			Repo: testRepo1,
			Files: map[string]string{
				"README.md": `# automation-testing
This repository is used to test opening and closing pull request with Automation

(c) Copyright Sourcegraph 2013-2020.
(c) Copyright Sourcegraph 2013-2020.
(c) Copyright Sourcegraph 2013-2020.`,
			},
		}

		cachedDiff := []byte(`diff --git README.md README.md
index 1914491..cd2ccbf 100644
--- README.md
+++ README.md
@@ -3,4 +3,5 @@ This repository is used to test opening and closing pull request with Automation

 (c) Copyright Sourcegraph 2013-2020.
 (c) Copyright Sourcegraph 2013-2020.
-(c) Copyright Sourcegraph 2013-2020.
\ No newline at end of file
+(c) Copyright Sourcegraph 2013-2020.this is step 2
+this is step 3
diff --git README.txt README.txt
new file mode 100644
index 0000000..888e1ec
--- /dev/null
+++ README.txt
@@ -0,0 +1 @@
+this is step 1
`)

		wantFinalDiff := `diff --git README.md README.md
index 1914491..d6782d3 100644
--- README.md
+++ README.md
@@ -3,4 +3,7 @@ This repository is used to test opening and closing pull request with Automation
 
 (c) Copyright Sourcegraph 2013-2020.
 (c) Copyright Sourcegraph 2013-2020.
-(c) Copyright Sourcegraph 2013-2020.
\ No newline at end of file
+(c) Copyright Sourcegraph 2013-2020.this is step 2
+this is step 3
+this is step 4
+previous_step.modified_files=[README.md]
diff --git README.txt README.txt
new file mode 100644
index 0000000..888e1ec
--- /dev/null
+++ README.txt
@@ -0,0 +1 @@
+this is step 1
diff --git my-output.txt my-output.txt
new file mode 100644
index 0000000..257ae8e
--- /dev/null
+++ my-output.txt
@@ -0,0 +1 @@
+this is step 5
`

		task := &Task{
			Repository:            testRepo1,
			BatchChangeAttributes: &template.BatchChangeAttributes{},
			Steps: []batcheslib.Step{
				{Run: `echo "this is step 1" >> README.txt`},
				{Run: `echo "this is step 2" >> README.md`},
				{Run: `echo "this is step 3" >> README.md`, Outputs: batcheslib.Outputs{
					"myOutput": batcheslib.Output{
						Value: "my-output.txt",
					},
				}},
				{Run: `echo "this is step 4" >> README.md
echo "previous_step.modified_files=${{ previous_step.modified_files }}" >> README.md
`},
				{Run: `echo "this is step 5" >> ${{ outputs.myOutput }}`},
			},
			CachedResultFound: true,
			CachedResult: stepExecutionResult{
				StepIndex: 2,
				Diff:      cachedDiff,
				Outputs: map[string]interface{}{
					"myOutput": "my-output.txt",
				},
				PreviousStepResult: template.StepResult{
					Files: &git.Changes{
						Modified: []string{"README.md"},
						Added:    []string{"README.txt"},
					},
					Stdout: nil,
					Stderr: nil,
				},
			},
		}

		results, err := testExecuteTasks(t, []*Task{task}, archive)
		if err != nil {
			t.Fatalf("execution failed: %s", err)
		}

		if have, want := len(results), 1; have != want {
			t.Fatalf("wrong number of execution results. want=%d, have=%d", want, have)
		}

		executionResult := results[0].result
		if diff := cmp.Diff(executionResult.Diff, wantFinalDiff); diff != "" {
			t.Fatalf("wrong diff: %s", diff)
		}

		if diff := cmp.Diff(executionResult.Outputs, task.CachedResult.Outputs); diff != "" {
			t.Fatalf("wrong execution result outputs: %s", diff)
		}

		// Only two steps should've been executed
		if have, want := len(results[0].stepResults), 2; have != want {
			t.Fatalf("wrong length of step results. have=%d, want=%d", have, want)
		}

		lastStepResult := results[0].stepResults[1]
		if have, want := lastStepResult.StepIndex, 4; have != want {
			t.Fatalf("wrong stepIndex. have=%d, want=%d", have, want)
		}

		if diff := cmp.Diff(lastStepResult.Outputs, task.CachedResult.Outputs); diff != "" {
			t.Fatalf("wrong step result outputs: %s", diff)
		}
	})
}

func testExecuteTasks(t *testing.T, tasks []*Task, archives ...mock.RepoArchive) ([]taskResult, error) {
	if runtime.GOOS == "windows" {
		t.Skip("Test doesn't work on Windows because dummydocker is written in bash")
	}

	testTempDir, err := ioutil.TempDir("", "executor-integration-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(testTempDir) })

	// Setup dummydocker
	addToPath(t, "testdata/dummydocker")

	// Setup mock test server & client
	mux := mock.NewZipArchivesMux(t, nil, archives...)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	var clientBuffer bytes.Buffer
	client := api.NewClient(api.ClientOpts{Endpoint: ts.URL, Out: &clientBuffer})

	// Prepare images
	//
	images := make(map[string]docker.Image)
	for _, t := range tasks {
		for _, step := range t.Steps {
			images[step.Container] = &mock.Image{RawDigest: step.Container}
		}
	}

	// Setup executor
	executor := newExecutor(newExecutorOpts{
		Creator:     workspace.NewCreator(context.Background(), "bind", testTempDir, testTempDir, images),
		Fetcher:     batches.NewRepoFetcher(client, testTempDir, false),
		Logger:      mock.LogNoOpManager{},
		EnsureImage: imageMapEnsurer(images),

		TempDir:     testTempDir,
		Parallelism: runtime.GOMAXPROCS(0),
		Timeout:     30 * time.Second,
	})

	executor.Start(context.Background(), tasks, newDummyTaskExecutionUI())
	return executor.Wait(context.Background())
}

func imageMapEnsurer(m map[string]docker.Image) imageEnsurer {
	return func(_ context.Context, container string) (docker.Image, error) {
		if i, ok := m[container]; ok {
			return i, nil
		}
		return nil, errors.New(fmt.Sprintf("image for %s not found", container))
	}
}

	"github.com/google/go-cmp/cmp"
	"github.com/sourcegraph/src-cli/internal/batches/git"
		{
			name: "cached result for step 3 of 5",
			archives: []mock.RepoArchive{
				{Repo: testRepo1, Files: map[string]string{
					"README.md": `# automation-testing
This repository is used to test opening and closing pull request with Automation

(c) Copyright Sourcegraph 2013-2020.
(c) Copyright Sourcegraph 2013-2020.
(c) Copyright Sourcegraph 2013-2020.`,
				}},
			},
			steps: []batches.Step{
				{Run: `echo "this is step 1" >> README.txt`},
				{Run: `echo "this is step 2" >> README.md`},
				{Run: `echo "this is step 3" >> README.md`, Outputs: batches.Outputs{
					"myOutput": batches.Output{
						Value: "my-output.txt",
					},
				}},
				{Run: `echo "this is step 4" >> README.md
echo "previous_step.modified_files=${{ previous_step.modified_files }}" >> README.md
`},
				{Run: `echo "this is step 5" >> ${{ outputs.myOutput }}`},
			},
			tasks: []*Task{
				{
					CachedResultFound: true,
					CachedResult: stepExecutionResult{
						StepIndex: 2,
						Diff: []byte(`diff --git README.md README.md
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
`),
						Outputs: map[string]interface{}{
							"myOutput": "my-output.txt",
						},
						PreviousStepResult: StepResult{
							Files: &git.Changes{
								Modified: []string{"README.md"},
								Added:    []string{"README.txt"},
							},
							Stdout: nil,
							Stderr: nil,
						},
					},
					Repository: testRepo1,
				},
			},
			wantFilesChanged: filesByRepository{
				testRepo1.ID: filesByPath{
					rootPath: []string{"README.md", "README.txt", "my-output.txt"},
				},
			},
		},
		{
			name: "cached result for step 0",
			archives: []mock.RepoArchive{
				{Repo: testRepo1, Files: map[string]string{
					"README.md": "# Welcome to the README\n",
				}},
			},
			steps: []batches.Step{
				{Run: `echo -e "foobar\n" >> README.md`},
			},
			tasks: []*Task{
				{
					CachedResultFound: true,
					CachedResult: stepExecutionResult{
						StepIndex: 0,
						Diff: []byte(`diff --git README.md README.md
index 02a19af..c9644dd 100644
--- README.md
+++ README.md
@@ -1 +1,2 @@
 # Welcome to the README
+foobar
`),
						Outputs:            map[string]interface{}{},
						PreviousStepResult: StepResult{},
					},
					Repository: testRepo1,
				},
			},
			wantFilesChanged: filesByRepository{
				testRepo1.ID: filesByPath{
					rootPath: []string{"README.md"},
				},
			},
		},
			statusHandler := NewTaskStatusCollection(tc.tasks)

			// Make sure that all the TaskStatus have been updated correctly
			statusHandler.CopyStatuses(func(statuses []*TaskStatus) {
				for i, status := range statuses {
					if status.StartedAt.IsZero() {
						t.Fatalf("status %d: StartedAt is zero", i)
					}
					if status.FinishedAt.IsZero() {
						t.Fatalf("status %d: FinishedAt is zero", i)
					}
					if status.CurrentlyExecuting != "" {
						t.Fatalf("status %d: CurrentlyExecuting not reset", i)
					}
				}
			})
		AllowOptionalPublished:   true,
	}
}

func TestExecutor_CachedStepResult_SingleStepCached(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Test doesn't work on Windows because dummydocker is written in bash")
	}

	// Temp dir for log files and downloaded archives
	testTempDir, err := ioutil.TempDir("", "executor-integration-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testTempDir)

	// Setup dummydocker
	addToPath(t, "testdata/dummydocker")

	// Setup mock test server & client
	archive := mock.RepoArchive{
		Repo: testRepo1, Files: map[string]string{
			"README.md": "# Welcome to the README\n",
		},
	}
	mux := mock.NewZipArchivesMux(t, nil, archive)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	var clientBuffer bytes.Buffer
	client := api.NewClient(api.ClientOpts{Endpoint: ts.URL, Out: &clientBuffer})

	// Setup Task with CachedResults
	cachedDiff := []byte(`diff --git README.md README.md
index 02a19af..c9644dd 100644
--- README.md
+++ README.md
@@ -1 +1,2 @@
 # Welcome to the README
+foobar
`)

	task := &Task{
		BatchChangeAttributes: &BatchChangeAttributes{},
		Steps: []batches.Step{
			{Run: `echo -e "foobar\n" >> README.md`},
		},
		CachedResultFound: true,
		CachedResult: stepExecutionResult{
			StepIndex:          0,
			Diff:               cachedDiff,
			Outputs:            map[string]interface{}{},
			PreviousStepResult: StepResult{},
		},
		Repository: testRepo1,
	}

	for i := range task.Steps {
		task.Steps[i].SetImage(&mock.Image{
			RawDigest: task.Steps[i].Container,
		})
	}

	// Setup executor
	executor := newExecutor(newExecutorOpts{
		Creator: workspace.NewCreator(context.Background(), "bind", testTempDir, testTempDir, []batches.Step{}),
		Fetcher: batches.NewRepoFetcher(client, testTempDir, false),
		Logger:  mock.LogNoOpManager{},

		TempDir:     testTempDir,
		Parallelism: runtime.GOMAXPROCS(0),
		Timeout:     30 * time.Second,
	})

	statusHandler := NewTaskStatusCollection([]*Task{task})

	// Run executor
	executor.Start(context.Background(), []*Task{task}, statusHandler)
	results, err := executor.Wait(context.Background())
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
package executor

import (
	"context"
	"io/ioutil"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/sourcegraph/src-cli/internal/batches"
	"github.com/sourcegraph/src-cli/internal/batches/git"
	"github.com/sourcegraph/src-cli/internal/batches/graphql"
	"github.com/sourcegraph/src-cli/internal/batches/log"
)

func TestCoordinator_Execute(t *testing.T) {
	template := &batches.ChangesetTemplate{
		Title:  "commit title",
		Body:   "commit body",
		Branch: "commit-branch",
		Commit: batches.ExpandedGitCommitDescription{
			Message: "commit msg",
			Author: &batches.GitCommitAuthor{
				Name:  "Tester",
				Email: "tester@example.com",
			},
		},
		Published: parsePublishedFieldString(t, "false"),
	}

	srcCLIRepo := &graphql.Repository{
		ID:            "src-cli",
		Name:          "github.com/sourcegraph/src-cli",
		DefaultBranch: &graphql.Branch{Name: "main", Target: graphql.Target{OID: "d34db33f"}},
	}

	srcCLITask := &Task{Repository: srcCLIRepo}

	sourcegraphRepo := &graphql.Repository{
		ID:   "sourcegraph",
		Name: "github.com/sourcegraph/sourcegraph",
		DefaultBranch: &graphql.Branch{
			Name:   "main",
			Target: graphql.Target{OID: "f00b4r3r"},
		},
	}

	sourcegraphTask := &Task{Repository: sourcegraphRepo}

	tests := []struct {
		name string

		executor *dummyExecutor

		tasks     []*Task
		batchSpec *batches.BatchSpec

		wantSpecs      []*batches.ChangesetSpec
		wantErrInclude string
	}{
		{
			name: "success",

			tasks: []*Task{srcCLITask, sourcegraphTask},

			batchSpec: &batches.BatchSpec{
				Name:              "my-batch-change",
				Description:       "the description",
				ChangesetTemplate: template,
			},

			executor: &dummyExecutor{
				results: []taskResult{
					{task: srcCLITask, result: executionResult{Diff: `dummydiff1`}},
					{task: sourcegraphTask, result: executionResult{Diff: `dummydiff2`}},
				},
			},

			wantSpecs: []*batches.ChangesetSpec{
				{
					BaseRepository: sourcegraphRepo.ID,
					CreatedChangeset: &batches.CreatedChangeset{
						BaseRef:        sourcegraphRepo.BaseRef(),
						BaseRev:        sourcegraphRepo.Rev(),
						HeadRepository: sourcegraphRepo.ID,
						HeadRef:        "refs/heads/" + template.Branch,
						Title:          template.Title,
						Body:           template.Body,
						Commits: []batches.GitCommitDescription{
							{
								Message:     template.Commit.Message,
								AuthorName:  template.Commit.Author.Name,
								AuthorEmail: template.Commit.Author.Email,
								Diff:        `dummydiff2`,
							},
						},
						Published: false,
					},
				},
				{
					BaseRepository: srcCLIRepo.ID,
					CreatedChangeset: &batches.CreatedChangeset{
						BaseRef:        srcCLIRepo.BaseRef(),
						BaseRev:        srcCLIRepo.Rev(),
						HeadRepository: srcCLIRepo.ID,
						HeadRef:        "refs/heads/" + template.Branch,
						Title:          template.Title,
						Body:           template.Body,
						Commits: []batches.GitCommitDescription{
							{
								Message:     template.Commit.Message,
								AuthorName:  template.Commit.Author.Name,
								AuthorEmail: template.Commit.Author.Email,
								Diff:        `dummydiff1`,
							},
						},
						Published: false,
					},
				},
			},
		},
		{
			name:  "templated changesetTemplate",
			tasks: []*Task{srcCLITask},

			batchSpec: &batches.BatchSpec{
				Name:        "my-batch-change",
				Description: "the description",
				ChangesetTemplate: &batches.ChangesetTemplate{
					Title: `output1=${{ outputs.output1}}`,
					Body: `output1=${{ outputs.output1}}
		output2=${{ outputs.output2.subField }}

		modified_files=${{ steps.modified_files }}
		added_files=${{ steps.added_files }}
		deleted_files=${{ steps.deleted_files }}
		renamed_files=${{ steps.renamed_files }}

		repository_name=${{ repository.name }}

		batch_change_name=${{ batch_change.name }}
		batch_change_description=${{ batch_change.description }}
		`,
					Branch: "templated-branch-${{ outputs.output1 }}",
					Commit: batches.ExpandedGitCommitDescription{
						Message: "output1=${{ outputs.output1}},output2=${{ outputs.output2.subField }}",
						Author: &batches.GitCommitAuthor{
							Name:  "output1=${{ outputs.output1}}",
							Email: "output1=${{ outputs.output1}}",
						},
					},
				},
			},

			executor: &dummyExecutor{
				results: []taskResult{
					{
						task: srcCLITask,
						result: executionResult{
							Diff: `dummydiff1`,
							Outputs: map[string]interface{}{
								"output1": "myOutputValue1",
								"output2": map[string]interface{}{
									"subField": "subFieldValue",
								},
							},
							ChangedFiles: &git.Changes{
								Modified: []string{"modified.txt"},
								Added:    []string{"added.txt"},
								Deleted:  []string{"deleted.txt"},
								Renamed:  []string{"renamed.txt"},
							},
						},
					},
				},
			},
			wantSpecs: []*batches.ChangesetSpec{
				{
					BaseRepository: srcCLIRepo.ID,
					CreatedChangeset: &batches.CreatedChangeset{
						BaseRef:        srcCLIRepo.BaseRef(),
						BaseRev:        srcCLIRepo.Rev(),
						HeadRepository: srcCLIRepo.ID,
						HeadRef:        "refs/heads/templated-branch-myOutputValue1",
						Title:          "output1=myOutputValue1",
						Body: `output1=myOutputValue1
		output2=subFieldValue

		modified_files=[modified.txt]
		added_files=[added.txt]
		deleted_files=[deleted.txt]
		renamed_files=[renamed.txt]

		repository_name=github.com/sourcegraph/src-cli

		batch_change_name=my-batch-change
		batch_change_description=the description`,
						Commits: []batches.GitCommitDescription{
							{
								Message:     "output1=myOutputValue1,output2=subFieldValue",
								AuthorName:  "output1=myOutputValue1",
								AuthorEmail: "output1=myOutputValue1",
								Diff:        `dummydiff1`,
							},
						},
						Published: false,
					},
				},
			},
		},
		// {
		// 	name: "transform group",
		// 	archives: []mock.RepoArchive{
		// 		{Repo: srcCLIRepo, Files: map[string]string{
		// 			"README.md":  "# Welcome to the README\n",
		// 			"a/a.go":     "package a",
		// 			"a/b/b.go":   "package b",
		// 			"a/b/c/c.go": "package c",
		// 		}},
		// 		{Repo: sourcegraphRepo, Files: map[string]string{
		// 			"README.md":  "# Welcome to the README\n",
		// 			"a/a.go":     "package a",
		// 			"a/b/b.go":   "package b",
		// 			"a/b/c/c.go": "package c",
		// 		}},
		// 	},
		//
		// 	tasks: []*Task{
		// 		{Repository: srcCLIRepo},
		// 		{Repository: sourcegraphRepo},
		// 	},
		// 	steps: []batches.Step{
		// 		{Run: `echo 'var a = 1' >> a/a.go`},
		// 		{Run: `echo 'var b = 2' >> a/b/b.go`},
		// 		{Run: `echo 'var c = 3' >> a/b/c/c.go`},
		// 	},
		// 	transform: &batches.TransformChanges{
		// 		Group: []batches.Group{
		// 			{Directory: "a/b/c", Branch: "in-directory-c"},
		// 			{Directory: "a/b", Branch: "in-directory-b", Repository: sourcegraphRepo.Name},
		// 		},
		// 	},
		//
		// 	wantFilesChanged: filesByRepository{
		// 		srcCLIRepo.ID: filesByBranch{
		// 			changesetTemplateBranch: []string{
		// 				"a/a.go",
		// 				"a/b/b.go",
		// 			},
		// 			"in-directory-c": []string{
		// 				"a/b/c/c.go",
		// 			},
		// 		},
		// 		sourcegraphRepo.ID: filesByBranch{
		// 			changesetTemplateBranch: []string{
		// 				"a/a.go",
		// 			},
		// 			"in-directory-b": []string{
		// 				"a/b/b.go",
		// 				"a/b/c/c.go",
		// 			},
		// 		},
		// 	},
		// },
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			// Set attributes on Task which would be set by the TaskBuilder
			for _, t := range tc.tasks {
				t.Template = tc.batchSpec.ChangesetTemplate
				t.BatchChangeAttributes = &BatchChangeAttributes{
					Name:        tc.batchSpec.Name,
					Description: tc.batchSpec.Description,
				}
			}

			testTempDir, err := ioutil.TempDir("", "executor-integration-test-*")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(testTempDir)

			logManager := log.NewManager(testTempDir, false)

			cache := newInMemoryExecutionCache()
			noopPrinter := func([]*TaskStatus) {}
			coord := Coordinator{
				cache:      cache,
				exec:       tc.executor,
				logManager: logManager,
			}

			// execute contains the actual logic for executing the tasks and
			// the batch spec. We'll run this multiple times to cover both the
			// cache and non-cache code paths.
			execute := func(t *testing.T) {
				specs, _, err := coord.Execute(ctx, tc.tasks, tc.batchSpec, noopPrinter)
				if tc.wantErrInclude == "" {
					if err != nil {
						t.Fatalf("execution failed: %s", err)
					}
				} else {
					if err == nil {
						t.Fatalf("expected error to include %q, but got no error", tc.wantErrInclude)
					} else {
						if !strings.Contains(err.Error(), tc.wantErrInclude) {
							t.Errorf("wrong error. have=%q want included=%q", err, tc.wantErrInclude)
						}
					}
					return
				}

				if have, want := len(specs), len(tc.wantSpecs); have != want {
					t.Fatalf("wrong number of changeset specs. want=%d, have=%d", want, have)
				}

				opts := []cmp.Option{
					cmpopts.EquateEmpty(),
					cmpopts.SortSlices(func(a, b *batches.ChangesetSpec) bool { return a.BaseRepository < b.BaseRepository }),
				}
				if !cmp.Equal(tc.wantSpecs, specs, opts...) {
					t.Errorf("wrong ChangesetSpecs (-want +got):\n%s", cmp.Diff(tc.wantSpecs, specs, opts...))
				}
			}

			verifyCache := func(t *testing.T) {
				want := len(tc.wantSpecs)
				if tc.wantErrInclude != "" {
					want = 0
				}

				// Verify that there is a cache entry for each repo.
				if have := cache.size(); have != want {
					t.Errorf("unexpected number of cache entries: have=%d want=%d cache=%+v", have, want, cache)
				}
			}

			// Sanity check, since we're going to be looking at the side effects
			// on the cache.
			if cache.size() != 0 {
				t.Fatalf("unexpectedly hot cache: %+v", cache)
			}

			// Run with a cold cache.
			t.Run("cold cache", func(t *testing.T) {
				execute(t)
				verifyCache(t)
			})

			// Run with a warm cache.
			t.Run("warm cache", func(t *testing.T) {
				execute(t)
				verifyCache(t)
			})
		})
	}
}

type dummyExecutor struct {
	results []taskResult
	waitErr error
}

func (d *dummyExecutor) Start(context.Context, []*Task, taskStatusHandler) {
	// "noop noop noop", the crowd screams
}

func (d *dummyExecutor) Wait(context.Context) ([]taskResult, error) {
	return d.results, d.waitErr
}

// inMemoryExecutionCache provides an in-memory cache for testing purposes.
type inMemoryExecutionCache struct {
	cache map[string]executionResult
	mu    sync.RWMutex
}

func newInMemoryExecutionCache() *inMemoryExecutionCache {
	return &inMemoryExecutionCache{
		cache: make(map[string]executionResult),
	}
}

func (c *inMemoryExecutionCache) size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.cache)
}

func (c *inMemoryExecutionCache) Get(ctx context.Context, key ExecutionCacheKey) (executionResult, bool, error) {
	k, err := key.Key()
	if err != nil {
		return executionResult{}, false, err
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	if res, ok := c.cache[k]; ok {
		return res, true, nil
	}
	return executionResult{}, false, nil
}

func (c *inMemoryExecutionCache) Set(ctx context.Context, key ExecutionCacheKey, result executionResult) error {
	k, err := key.Key()
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache[k] = result
	return nil
}

func (c *inMemoryExecutionCache) Clear(ctx context.Context, key ExecutionCacheKey) error {
	k, err := key.Key()
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.cache, k)
	return nil
}

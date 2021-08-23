package service

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strings"

	"github.com/pkg/errors"
	batcheslib "github.com/sourcegraph/sourcegraph/lib/batches"
	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/batches"
	"github.com/sourcegraph/src-cli/internal/batches/docker"
	"github.com/sourcegraph/src-cli/internal/batches/executor"
	"github.com/sourcegraph/src-cli/internal/batches/graphql"
)

type Service struct {
	allowUnsupported bool
	allowIgnored     bool
	client           api.Client
	features         batches.FeatureFlags
	imageCache       *docker.ImageCache
}

type Opts struct {
	AllowUnsupported bool
	AllowIgnored     bool
	Client           api.Client
}

var (
	ErrMalformedOnQueryOrRepository = errors.New("malformed 'on' field; missing either a repository name or a query")
)

func New(opts *Opts) *Service {
	return &Service{
		allowUnsupported: opts.AllowUnsupported,
		allowIgnored:     opts.AllowIgnored,
		client:           opts.Client,
		imageCache:       docker.NewImageCache(),
	}
}

const sourcegraphVersionQuery = `query SourcegraphVersion {
	site {
	  productVersion
	}
  }
  `

// getSourcegraphVersion queries the Sourcegraph GraphQL API to get the
// current version of the Sourcegraph instance.
func (svc *Service) getSourcegraphVersion(ctx context.Context) (string, error) {
	var result struct {
		Site struct {
			ProductVersion string
		}
	}

	ok, err := svc.client.NewQuery(sourcegraphVersionQuery).Do(ctx, &result)
	if err != nil || !ok {
		return "", err
	}

	return result.Site.ProductVersion, err
}

// DetermineFeatureFlags fetches the version of the configured Sourcegraph
// instance and then sets flags on the Service itself to use features available
// in that version, e.g. gzip compression.
func (svc *Service) DetermineFeatureFlags(ctx context.Context) error {
	version, err := svc.getSourcegraphVersion(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to query Sourcegraph version to check for available features")
	}

	return svc.features.SetFromVersion(version)
}

// TODO(campaigns-deprecation): this shim can be removed in Sourcegraph 4.0.
func (svc *Service) newOperations() graphql.Operations {
	return graphql.NewOperations(
		svc.client,
		svc.features.BatchChanges,
		svc.features.UseGzipCompression,
	)
}

func (svc *Service) newRequest(query string, vars map[string]interface{}) api.Request {
	if svc.features.UseGzipCompression {
		return svc.client.NewGzippedRequest(query, vars)
	}
	return svc.client.NewRequest(query, vars)
}

func (svc *Service) ApplyBatchChange(ctx context.Context, spec graphql.BatchSpecID) (*graphql.BatchChange, error) {
	return svc.newOperations().ApplyBatchChange(ctx, spec)
}

func (svc *Service) CreateBatchSpec(ctx context.Context, namespace, spec string, ids []graphql.ChangesetSpecID) (graphql.BatchSpecID, string, error) {
	result, err := svc.newOperations().CreateBatchSpec(ctx, namespace, spec, ids)
	if err != nil {
		return "", "", err
	}

	return result.ID, result.ApplyURL, nil
}

const createChangesetSpecMutation = `
mutation CreateChangesetSpec($spec: String!) {
    createChangesetSpec(changesetSpec: $spec) {
        ... on HiddenChangesetSpec {
            id
        }
        ... on VisibleChangesetSpec {
            id
        }
    }
}
`

func (svc *Service) CreateChangesetSpec(ctx context.Context, spec *batches.ChangesetSpec) (graphql.ChangesetSpecID, error) {
	raw, err := json.Marshal(spec)
	if err != nil {
		return "", errors.Wrap(err, "marshalling changeset spec JSON")
	}

	var result struct {
		CreateChangesetSpec struct {
			ID string
		}
	}
	if ok, err := svc.newRequest(createChangesetSpecMutation, map[string]interface{}{
		"spec": string(raw),
	}).Do(ctx, &result); err != nil || !ok {
		return "", err
	}

	return graphql.ChangesetSpecID(result.CreateChangesetSpec.ID), nil
}

// EnsureDockerImages iterates over the steps within the batch spec to ensure the
// images exist and to determine the exact content digest to be used when running
// each step, including any required by the service itself.
//
// Progress information is reported back to the given progress function: perc
// will be a value between 0.0 and 1.0, inclusive.
func (svc *Service) EnsureDockerImages(ctx context.Context, spec *batcheslib.BatchSpec, progress func(perc float64)) (map[string]docker.Image, error) {
	total := len(spec.Steps) + 1
	progress(0)

	// TODO: this _really_ should be parallelised, since the image cache takes
	// care to only pull the same image once.
	images := make(map[string]docker.Image)
	for i := range spec.Steps {
		img, err := svc.EnsureImage(ctx, spec.Steps[i].Container)
		if err != nil {
			return nil, err
		}
		images[spec.Steps[i].Container] = img

		progress(float64(i) / float64(total))
	}

	progress(1)
	return images, nil
}

func (svc *Service) EnsureImage(ctx context.Context, name string) (docker.Image, error) {
	img := svc.imageCache.Get(name)

	if err := img.Ensure(ctx); err != nil {
		return nil, errors.Wrapf(err, "pulling image %q", name)
	}

	return img, nil
}

func (svc *Service) DetermineWorkspaces(ctx context.Context, repos []*graphql.Repository, spec *batcheslib.BatchSpec) ([]RepoWorkspaces, error) {
	return findWorkspaces(ctx, spec, svc, repos)
}

func (svc *Service) BuildTasks(ctx context.Context, repos []*graphql.Repository, spec *batcheslib.BatchSpec, workspaces []RepoWorkspaces) ([]*executor.Task, error) {
	return buildTasks(ctx, spec, svc, repos, workspaces)
}

func (svc *Service) NewCoordinator(opts executor.NewCoordinatorOpts) *executor.Coordinator {
	opts.ResolveRepoName = svc.resolveRepositoryName
	opts.Client = svc.client
	opts.Features = svc.features
	opts.EnsureImage = svc.EnsureImage

	return executor.NewCoordinator(opts)
}

// ValidateChangesetSpecs validates that among all branch changesets there are no
// duplicates in branch names in a single repo.
func (svc *Service) ValidateChangesetSpecs(repos []*graphql.Repository, specs []*batches.ChangesetSpec) error {
	repoByID := make(map[string]*graphql.Repository, len(repos))
	for _, repo := range repos {
		repoByID[repo.ID] = repo
	}

	byRepoAndBranch := make(map[string]map[string][]*batches.ChangesetSpec)
	for _, spec := range specs {
		// We don't need to validate imported changesets, as they can
		// never have a critical branch name overlap.
		if spec.ExternalChangeset != nil {
			continue
		}
		if _, ok := byRepoAndBranch[spec.HeadRepository]; !ok {
			byRepoAndBranch[spec.HeadRepository] = make(map[string][]*batches.ChangesetSpec)
		}

		byRepoAndBranch[spec.HeadRepository][spec.HeadRef] = append(byRepoAndBranch[spec.HeadRepository][spec.HeadRef], spec)
	}

	duplicates := make(map[*graphql.Repository]map[string]int)
	for repoID, specsByBranch := range byRepoAndBranch {
		for branch, specs := range specsByBranch {
			if len(specs) < 2 {
				continue
			}

			r := repoByID[repoID]
			if _, ok := duplicates[r]; !ok {
				duplicates[r] = make(map[string]int)
			}

			duplicates[r][branch] = len(specs)
		}
	}

	if len(duplicates) > 0 {
		return &duplicateBranchesErr{duplicates: duplicates}
	}

	return nil
}

type duplicateBranchesErr struct {
	duplicates map[*graphql.Repository]map[string]int
}

func (e *duplicateBranchesErr) Error() string {
	var out strings.Builder

	fmt.Fprintf(&out, "Multiple changeset specs have the same branch:\n\n")

	for repo, branches := range e.duplicates {
		for branch, duplicates := range branches {
			branch = strings.TrimPrefix(branch, "refs/heads/")
			fmt.Fprintf(&out, "\t* %s: %d changeset specs have the branch %q\n", repo.Name, duplicates, branch)
		}
	}

	fmt.Fprint(&out, "\nMake sure that the changesetTemplate.branch field in the batch spec produces unique values for each changeset in a single repository and rerun this command.")

	return out.String()
}

func (svc *Service) ParseBatchSpec(data []byte) (*batcheslib.BatchSpec, error) {
	spec, err := batcheslib.ParseBatchSpec(data, batcheslib.ParseBatchSpecOptions{
		AllowArrayEnvironments: svc.features.AllowArrayEnvironments,
		AllowTransformChanges:  svc.features.AllowTransformChanges,
		AllowConditionalExec:   svc.features.AllowConditionalExec,
	})
	if err != nil {
		return nil, errors.Wrap(err, "parsing batch spec")
	}
	return spec, nil
}

const exampleSpecTmpl = `name: NAME-OF-YOUR-BATCH-CHANGE
description: DESCRIPTION-OF-YOUR-BATCH-CHANGE

# "on" specifies on which repositories to execute the "steps".
on:
  # Example: find all repositories that contain a README.md file.
  - repositoriesMatchingQuery: file:README.md

# "steps" are run in each repository. Each step is run in a Docker container
# with the repository as the working directory. Once complete, each
# repository's resulting diff is captured.
steps:
  # Example: append "Hello World" to every README.md
  - run: echo "Hello World" | tee -a $(find -name README.md)
    container: alpine:3

# "changesetTemplate" describes the changeset (e.g., GitHub pull request) that
# will be created for each repository.
changesetTemplate:
  title: Hello World
  body: This adds Hello World to the README

  branch: BRANCH-NAME-IN-EACH-REPOSITORY # Push the commit to this branch.

  commit:
    author:
      name: {{ .Author.Name }}
      email: {{ .Author.Email }}
    message: Append Hello World to all README.md files
`

const exampleSpecPublishFlagTmpl = `
  # Change published to true once you're ready to create changesets on the code host.
  published: false
`

func (svc *Service) GenerateExampleSpec(ctx context.Context, fileName string) error {
	// Try to create file. Bail out, if it already exists.
	f, err := os.OpenFile(fileName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("file %s already exists", fileName)
		}
		return errors.Wrapf(err, "failed to create file %s", fileName)
	}
	defer f.Close()

	t := exampleSpecTmpl
	if !svc.features.AllowOptionalPublished {
		t += exampleSpecPublishFlagTmpl
	}
	tmpl, err := template.New("").Parse(t)
	if err != nil {
		return err
	}

	author := batcheslib.GitCommitAuthor{
		Name:  "Sourcegraph",
		Email: "batch-changes@sourcegraph.com",
	}
	// Try to get better default values from git, ignore any errors.
	gitAuthorName, err1 := getGitConfig("user.name")
	gitAuthorEmail, err2 := getGitConfig("user.email")
	if err1 == nil && err2 == nil && gitAuthorName != "" && gitAuthorEmail != "" {
		author.Name = gitAuthorName
		author.Email = gitAuthorEmail
	}

	err = tmpl.Execute(f, map[string]interface{}{"Author": author})
	if err != nil {
		return errors.Wrap(err, "failed to write batch spec to file")
	}

	return nil
}

const namespaceQuery = `
query NamespaceQuery($name: String!) {
    user(username: $name) {
        id
    }

    organization(name: $name) {
        id
    }
}
`

const usernameQuery = `
query GetCurrentUserID {
    currentUser {
        id
    }
}
`

func (svc *Service) ResolveNamespace(ctx context.Context, namespace string) (string, error) {
	if namespace == "" {
		// if no namespace is provided, default to logged in user as namespace
		var resp struct {
			Data struct {
				CurrentUser struct {
					ID string `json:"id"`
				} `json:"currentUser"`
			} `json:"data"`
		}
		if ok, err := svc.client.NewRequest(usernameQuery, nil).DoRaw(ctx, &resp); err != nil || !ok {
			return "", errors.WithMessage(err, "failed to resolve namespace: no user logged in")
		}

		if resp.Data.CurrentUser.ID == "" {
			return "", errors.New("cannot resolve current user")
		}
		return resp.Data.CurrentUser.ID, nil
	}

	var result struct {
		Data struct {
			User         *struct{ ID string }
			Organization *struct{ ID string }
		}
		Errors []interface{}
	}
	if ok, err := svc.client.NewRequest(namespaceQuery, map[string]interface{}{
		"name": namespace,
	}).DoRaw(ctx, &result); err != nil || !ok {
		return "", err
	}

	if result.Data.User != nil {
		return result.Data.User.ID, nil
	}
	if result.Data.Organization != nil {
		return result.Data.Organization.ID, nil
	}
	return "", fmt.Errorf("failed to resolve namespace %q: no user or organization found", namespace)
}

func (svc *Service) ResolveRepositories(ctx context.Context, spec *batcheslib.BatchSpec) ([]*graphql.Repository, error) {
	seen := map[string]*graphql.Repository{}
	unsupported := batches.UnsupportedRepoSet{}
	ignored := batches.IgnoredRepoSet{}

	// TODO: this could be trivially parallelised in the future.
	for _, on := range spec.On {
		repos, err := svc.ResolveRepositoriesOn(ctx, &on)
		if err != nil {
			return nil, errors.Wrapf(err, "resolving %q", on.String())
		}

		var repoBatchIgnores map[*graphql.Repository][]string
		if !svc.allowIgnored {
			repoBatchIgnores, err = svc.FindDirectoriesInRepos(ctx, ".batchignore", repos...)
			if err != nil {
				return nil, err
			}
		}

		for _, repo := range repos {
			if !repo.HasBranch() {
				continue
			}

			if other, ok := seen[repo.ID]; !ok {
				seen[repo.ID] = repo

				switch st := strings.ToLower(repo.ExternalRepository.ServiceType); st {
				case "github", "gitlab", "bitbucketserver":
				default:
					if !svc.allowUnsupported {
						unsupported.Append(repo)
					}
				}

				if !svc.allowIgnored {
					if locations, ok := repoBatchIgnores[repo]; ok && len(locations) > 0 {
						ignored.Append(repo)
					}
				}
			} else {
				// If we've already seen this repository, we overwrite the
				// Commit/Branch fields with the latest value we have
				other.Commit = repo.Commit
				other.Branch = repo.Branch
			}
		}
	}

	final := make([]*graphql.Repository, 0, len(seen))
	for _, repo := range seen {
		if !unsupported.Includes(repo) && !ignored.Includes(repo) {
			final = append(final, repo)
		}
	}

	if unsupported.HasUnsupported() {
		return final, unsupported
	}

	if ignored.HasIgnored() {
		return final, ignored
	}

	return final, nil
}

func (svc *Service) ResolveRepositoriesOn(ctx context.Context, on *batcheslib.OnQueryOrRepository) ([]*graphql.Repository, error) {
	if on.RepositoriesMatchingQuery != "" {
		return svc.resolveRepositorySearch(ctx, on.RepositoriesMatchingQuery)
	} else if on.Repository != "" && on.Branch != "" {
		repo, err := svc.resolveRepositoryNameAndBranch(ctx, on.Repository, on.Branch)
		if err != nil {
			return nil, err
		}
		return []*graphql.Repository{repo}, nil
	} else if on.Repository != "" {
		repo, err := svc.resolveRepositoryName(ctx, on.Repository)
		if err != nil {
			return nil, err
		}
		return []*graphql.Repository{repo}, nil
	}

	// This shouldn't happen on any batch spec that has passed validation, but,
	// alas, software.
	return nil, ErrMalformedOnQueryOrRepository
}

const repositoryNameQuery = `
query Repository($name: String!, $queryCommit: Boolean!, $rev: String!) {
    repository(name: $name) {
        ...repositoryFields
    }
}
` + graphql.RepositoryFieldsFragment

func (svc *Service) resolveRepositoryName(ctx context.Context, name string) (*graphql.Repository, error) {
	var result struct{ Repository *graphql.Repository }
	if ok, err := svc.client.NewRequest(repositoryNameQuery, map[string]interface{}{
		"name":        name,
		"queryCommit": false,
		"rev":         "",
	}).Do(ctx, &result); err != nil || !ok {
		return nil, err
	}
	if result.Repository == nil {
		return nil, errors.New("no repository found")
	}
	return result.Repository, nil
}

func (svc *Service) resolveRepositoryNameAndBranch(ctx context.Context, name, branch string) (*graphql.Repository, error) {
	var result struct{ Repository *graphql.Repository }
	if ok, err := svc.client.NewRequest(repositoryNameQuery, map[string]interface{}{
		"name":        name,
		"queryCommit": true,
		"rev":         branch,
	}).Do(ctx, &result); err != nil || !ok {
		return nil, err
	}
	if result.Repository == nil {
		return nil, errors.New("no repository found")
	}
	if result.Repository.Commit.OID == "" {
		return nil, fmt.Errorf("no branch matching %q found for repository %s", branch, name)
	}

	result.Repository.Branch = graphql.Branch{
		Name:   branch,
		Target: result.Repository.Commit,
	}

	return result.Repository, nil
}

// TODO: search result alerts.
const repositorySearchQuery = `
query ChangesetRepos(
    $query: String!,
	$queryCommit: Boolean!,
	$rev: String!,
) {
    search(query: $query, version: V2) {
        results {
            results {
                __typename
                ... on Repository {
                    ...repositoryFields
                }
                ... on FileMatch {
                    file { path }
                    repository {
                        ...repositoryFields
                    }
                }
            }
        }
    }
}
` + graphql.RepositoryFieldsFragment

func (svc *Service) resolveRepositorySearch(ctx context.Context, query string) ([]*graphql.Repository, error) {
	var result struct {
		Search struct {
			Results struct {
				Results []searchResult
			}
		}
	}

	if ok, err := svc.client.NewRequest(repositorySearchQuery, map[string]interface{}{
		"query":       setDefaultQueryCount(query),
		"queryCommit": false,
		"rev":         "",
	}).Do(ctx, &result); err != nil || !ok {
		return nil, err
	}

	ids := map[string]*graphql.Repository{}
	var repos []*graphql.Repository
	for _, r := range result.Search.Results.Results {
		existing, ok := ids[r.ID]
		if !ok {
			repo := r.Repository
			repos = append(repos, &repo)
			ids[r.ID] = &repo
		} else {
			for file := range r.FileMatches {
				existing.FileMatches[file] = true
			}
		}
	}
	return repos, nil
}

// findDirectoriesResult maps the name of the GraphQL query to its results. The
// name is the repository's ID.
type findDirectoriesResult map[string]struct {
	Results struct{ Results []searchResult }
}

const searchQueryTmpl = `%s: search(query: %q, version: V2) {
	results {
		results {
			__typename
			... on FileMatch {
				file { path }
			}
		}
	}
}
`

const findDirectoriesInReposBatchSize = 50

// FindDirectoriesInRepos returns a map of repositories and the locations of
// files matching the given file name in the repository.
// The locations are paths relative to the root of the directory.
// No "/" at the beginning.
// A dot (".") represents the root directory.
func (svc *Service) FindDirectoriesInRepos(ctx context.Context, fileName string, repos ...*graphql.Repository) (map[*graphql.Repository][]string, error) {
	// Build up unique identifiers that are safe to use as GraphQL query aliases.
	reposByQueryID := map[string]*graphql.Repository{}
	queryIDByRepo := map[*graphql.Repository]string{}
	for i, repo := range repos {
		queryID := fmt.Sprintf("repo_%d", i)
		reposByQueryID[queryID] = repo
		queryIDByRepo[repo] = queryID
	}

	findInBatch := func(batch []*graphql.Repository, results map[*graphql.Repository][]string) error {
		var a strings.Builder
		a.WriteString("query DirectoriesContainingFile {\n")

		for _, repo := range batch {
			query := fmt.Sprintf(`file:(^|/)%s$ repo:^%s$ type:path count:99999`, regexp.QuoteMeta(fileName), regexp.QuoteMeta(repo.Name))

			a.WriteString(fmt.Sprintf(searchQueryTmpl, queryIDByRepo[repo], query))
		}

		a.WriteString("}")

		var result findDirectoriesResult
		if ok, err := svc.client.NewQuery(a.String()).Do(ctx, &result); err != nil || !ok {
			return err
		}

		for queryID, search := range result {
			repo, ok := reposByQueryID[queryID]
			if !ok {
				return fmt.Errorf("result for query %q did not match any repository", queryID)
			}

			files := map[string]struct{}{}

			for _, r := range search.Results.Results {
				for file := range r.FileMatches {
					files[file] = struct{}{}
				}
			}

			var dirs []string
			for f := range files {
				// We use path.Dir and not filepath.Dir here, because while
				// src-cli might be executed on Windows, we need the paths to
				// be Unix paths, since they will be used inside Docker
				// containers.
				dirs = append(dirs, path.Dir(f))
			}

			results[repo] = dirs
		}

		return nil
	}

	results := make(map[*graphql.Repository][]string)

	for start := 0; start < len(repos); start += findDirectoriesInReposBatchSize {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		end := start + findDirectoriesInReposBatchSize
		if end > len(repos) {
			end = len(repos)
		}

		batch := repos[start:end]

		err := findInBatch(batch, results)
		if err != nil {
			return results, err
		}
	}

	return results, nil
}

var defaultQueryCountRegex = regexp.MustCompile(`\bcount:(\d+|all)\b`)

const hardCodedCount = " count:999999"

func setDefaultQueryCount(query string) string {
	if defaultQueryCountRegex.MatchString(query) {
		return query
	}

	return query + hardCodedCount
}

type searchResult struct {
	graphql.Repository
}

func (sr *searchResult) UnmarshalJSON(data []byte) error {
	var tn struct {
		Typename string `json:"__typename"`
	}
	if err := json.Unmarshal(data, &tn); err != nil {
		return err
	}

	switch tn.Typename {
	case "FileMatch":
		var result struct {
			Repository graphql.Repository
			File       struct {
				Path string
			}
		}
		if err := json.Unmarshal(data, &result); err != nil {
			return err
		}

		sr.Repository = result.Repository
		sr.Repository.FileMatches = map[string]bool{result.File.Path: true}
		return nil

	case "Repository":
		if err := json.Unmarshal(data, &sr.Repository); err != nil {
			return err
		}
		sr.Repository.FileMatches = map[string]bool{}
		return nil

	default:
		return errors.Errorf("unknown GraphQL type %q", tn.Typename)
	}
}

func getGitConfig(attribute string) (string, error) {
	cmd := exec.Command("git", "config", "--get", attribute)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

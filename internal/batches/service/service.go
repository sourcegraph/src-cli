package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"text/template"

	batcheslib "github.com/sourcegraph/sourcegraph/lib/batches"
	templatelib "github.com/sourcegraph/sourcegraph/lib/batches/template"
	"github.com/sourcegraph/sourcegraph/lib/errors"

	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/batches"
	"github.com/sourcegraph/src-cli/internal/batches/docker"
	"github.com/sourcegraph/src-cli/internal/batches/executor"
	"github.com/sourcegraph/src-cli/internal/batches/graphql"
)

type Service struct {
	client   api.Client
	features batches.FeatureFlags
}

type Opts struct {
	Client api.Client
}

var (
	ErrMalformedOnQueryOrRepository = errors.New("malformed 'on' field; missing either a repository name or a query")
)

func New(opts *Opts) *Service {
	return &Service{
		client: opts.Client,
	}
}

// The reason we ask for batchChanges here is to surface errors about trying to use batch
// changes in an unsupported environment sooner, since the version check is typically the
// first thing we do.
const sourcegraphVersionQuery = `query SourcegraphVersion {
	site {
		productVersion
	}
	batchChanges(first: 1) {
		nodes {
			id
		}
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

	return svc.SetFeatureFlagsForVersion(version)
}

func (svc *Service) SetFeatureFlagsForVersion(version string) error {
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

func (svc *Service) CreateChangesetSpec(ctx context.Context, spec *batcheslib.ChangesetSpec) (graphql.ChangesetSpecID, error) {
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

const resolveWorkspacesForBatchSpecQuery = `
query ResolveWorkspacesForBatchSpec($spec: String!) {
    resolveWorkspacesForBatchSpec(batchSpec: $spec) {
		onlyFetchWorkspace
		ignored
		unsupported
		repository {
			id
			name
			url
			externalRepository { serviceType }
			defaultBranch {
				name
				target { oid }
			}
		}
		branch {
			name
			target {
				oid
			}
		}
		path
		searchResultPaths
    }
}
`

func (svc *Service) ResolveWorkspacesForBatchSpec(ctx context.Context, spec *batcheslib.BatchSpec, allowUnsupported, allowIgnored bool) ([]RepoWorkspace, []*graphql.Repository, error) {
	raw, err := json.Marshal(spec)
	if err != nil {
		return nil, nil, errors.Wrap(err, "marshalling changeset spec JSON")
	}

	var result struct {
		ResolveWorkspacesForBatchSpec []struct {
			OnlyFetchWorkspace bool
			Ignored            bool
			Unsupported        bool
			Repository         *graphql.Repository
			Branch             *graphql.Branch
			Path               string
			SearchResultPaths  []string
		}
	}
	if ok, err := svc.newRequest(resolveWorkspacesForBatchSpecQuery, map[string]interface{}{
		"spec": string(raw),
	}).Do(ctx, &result); err != nil || !ok {
		return nil, nil, err
	}

	unsupported := batches.UnsupportedRepoSet{}
	ignored := batches.IgnoredRepoSet{}

	repos := make([]*graphql.Repository, 0, len(result.ResolveWorkspacesForBatchSpec))
	seenRepos := make(map[string]struct{})
	workspaces := make([]RepoWorkspace, 0, len(result.ResolveWorkspacesForBatchSpec))
	for _, w := range result.ResolveWorkspacesForBatchSpec {
		fileMatches := make(map[string]bool)
		for _, path := range w.SearchResultPaths {
			fileMatches[path] = true
		}

		workspace := RepoWorkspace{
			Repo: &graphql.Repository{
				ID:                 w.Repository.ID,
				Name:               w.Repository.Name,
				URL:                w.Repository.URL,
				FileMatches:        fileMatches,
				ExternalRepository: w.Repository.ExternalRepository,
				DefaultBranch:      w.Repository.DefaultBranch,
				Commit:             w.Branch.Target,
				Branch:             *w.Branch,
			},
			Path:               w.Path,
			OnlyFetchWorkspace: w.OnlyFetchWorkspace,
		}

		if !allowIgnored && w.Ignored {
			ignored.Append(workspace.Repo)
		} else if !allowUnsupported && w.Unsupported {
			unsupported.Append(workspace.Repo)
		} else {
			// Collect the repo, if not seen yet.
			if _, ok := seenRepos[workspace.Repo.ID]; !ok {
				seenRepos[workspace.Repo.ID] = struct{}{}
				repos = append(repos, workspace.Repo)
			}

			workspaces = append(workspaces, workspace)
		}
	}

	if unsupported.HasUnsupported() {
		return workspaces, repos, unsupported
	}

	if ignored.HasIgnored() {
		return workspaces, repos, ignored
	}

	return workspaces, repos, nil
}

// EnsureDockerImages iterates over the steps within the batch spec to ensure the
// images exist and to determine the exact content digest to be used when running
// each step, including any required by the service itself.
//
// Progress information is reported back to the given progress function.
func (svc *Service) EnsureDockerImages(
	ctx context.Context,
	imageCache docker.ImageCache,
	steps []batcheslib.Step,
	parallelism int,
	progress func(done, total int),
) (map[string]docker.Image, error) {
	// Figure out the image names used in the batch spec.
	names := map[string]struct{}{}
	for i := range steps {
		names[steps[i].Container] = struct{}{}
	}

	total := len(names)
	progress(0, total)

	// Set up the channels that will be used in the parallel goroutines handling
	// the pulls.
	type image struct {
		name  string
		image docker.Image
		err   error
	}
	complete := make(chan image)
	inputs := make(chan string, total)

	// Set up a worker context that we can use to terminate the workers if an
	// error occurs.
	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Spawn worker goroutines to call EnsureImage on each image name.
	if parallelism < 1 {
		parallelism = 1
	}
	if parallelism > total {
		parallelism = total
	}
	var wg sync.WaitGroup
	for i := 0; i < parallelism; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-workerCtx.Done():
					// If the worker context has been cancelled, then we just want to
					// return immediately, rather than continuing to read from inputs.
					return
				case name, more := <-inputs:
					if !more {
						return
					}
					img, err := imageCache.Ensure(workerCtx, name)
					select {
					case <-workerCtx.Done():
						return
					case complete <- image{
						name:  name,
						image: img,
						err:   err,
					}:
						// All good; let's move onto the next input.
					}
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(complete)
	}()

	// Send the image names to the worker goroutines.
	go func() {
		for name := range names {
			inputs <- name
		}
		close(inputs)
	}()

	// Receive the results of the image pulls and build the return value.
	i := 0
	images := make(map[string]docker.Image)
	for image := range complete {
		if image.err != nil {
			// If EnsureImage errored, then we'll early return here and let the
			// worker context clean things up.
			return nil, image.err
		}

		images[image.name] = image.image
		i += 1
		progress(i, total)
	}

	return images, nil
}

func (svc *Service) BuildTasks(attributes *templatelib.BatchChangeAttributes, steps []batcheslib.Step, workspaces []RepoWorkspace) []*executor.Task {
	return buildTasks(attributes, steps, workspaces)
}

func (svc *Service) Features() batches.FeatureFlags {
	return svc.features
}

func (svc *Service) CreateImportChangesetSpecs(ctx context.Context, batchSpec *batcheslib.BatchSpec) ([]*batcheslib.ChangesetSpec, error) {
	return batcheslib.BuildImportChangesetSpecs(ctx, batchSpec.ImportChangesets, func(ctx context.Context, repoNames []string) (_ map[string]string, errs error) {
		repoNameIDs := map[string]string{}
		for _, name := range repoNames {
			repo, err := svc.resolveRepositoryName(ctx, name)
			if err != nil {
				wrapped := errors.Wrapf(err, "resolving repository name %q", name)
				errs = errors.Append(errs, wrapped)
				continue
			}
			repoNameIDs[name] = repo.ID
		}
		return repoNameIDs, errs
	})
}

// ValidateChangesetSpecs validates that among all branch changesets there are no
// duplicates in branch names in a single repo.
func (svc *Service) ValidateChangesetSpecs(repos []*graphql.Repository, specs []*batcheslib.ChangesetSpec) error {
	repoByID := make(map[string]*graphql.Repository, len(repos))
	for _, repo := range repos {
		repoByID[repo.ID] = repo
	}

	byRepoAndBranch := make(map[string]map[string][]*batcheslib.ChangesetSpec)
	for _, spec := range specs {
		// We don't need to validate imported changesets, as they can
		// never have a critical branch name overlap.
		if spec.Type() == batcheslib.ChangesetSpecDescriptionTypeExisting {
			continue
		}
		if _, ok := byRepoAndBranch[spec.HeadRepository]; !ok {
			byRepoAndBranch[spec.HeadRepository] = make(map[string][]*batcheslib.ChangesetSpec)
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

func (svc *Service) ParseBatchSpec(dir string, data []byte, isRemote bool) (*batcheslib.BatchSpec, error) {
	spec, err := batcheslib.ParseBatchSpec(data, batcheslib.ParseBatchSpecOptions{
		AllowArrayEnvironments: svc.features.AllowArrayEnvironments,
		AllowTransformChanges:  svc.features.AllowTransformChanges,
		AllowConditionalExec:   svc.features.AllowConditionalExec,
	})
	if err != nil {
		return nil, errors.Wrap(err, "parsing batch spec")
	}
	if err = validateMount(dir, spec, isRemote); err != nil {
		return nil, errors.Wrap(err, "handling mount")
	}
	return spec, nil
}

func validateMount(batchSpecDir string, spec *batcheslib.BatchSpec, isRemote bool) error {
	for i, step := range spec.Steps {
		for j, mount := range step.Mount {
			if isRemote {
				return errors.New("mounts are not supported for server-side processing")
			}
			p := mount.Path
			if !filepath.IsAbs(p) {
				// Try to build the absolute path since Docker will only mount absolute paths
				p = filepath.Join(batchSpecDir, p)
			}
			pathInfo, err := os.Stat(p)
			if os.IsNotExist(err) {
				return errors.Newf("step %d mount path %s does not exist", i+1, p)
			} else if err != nil {
				return errors.Wrapf(err, "step %d mount path validation", i+1)
			}
			if !strings.HasPrefix(p, batchSpecDir) {
				return errors.Newf("step %d mount path is not in the same directory or subdirectory as the batch spec", i+1)
			}
			// Mounting a directory on Docker must end with the separator. So, append the file separator to make
			// users' lives easier.
			if pathInfo.IsDir() && !strings.HasSuffix(p, string(filepath.Separator)) {
				p += string(filepath.Separator)
			}
			// Update the mount path to the absolute path so building the absolute path (above) does not need to be
			// redone when adding the mount argument to the Docker container.
			// TODO: Can this mess with caching? We wouldn't be doing that server-side.
			step.Mount[j].Path = p
		}
	}
	return nil
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
        url
    }

    organization(name: $name) {
        id
        url
    }
}
`

const usernameQuery = `
query GetCurrentUserID {
    currentUser {
        id
        url
    }
}
`

type Namespace struct {
	ID  string
	URL string
}

func (svc *Service) ResolveNamespace(ctx context.Context, namespace string) (Namespace, error) {
	if namespace == "" {
		// if no namespace is provided, default to logged in user as namespace
		var resp struct {
			Data struct {
				CurrentUser struct {
					ID  string `json:"id"`
					URL string `json:"url"`
				} `json:"currentUser"`
			} `json:"data"`
		}
		if ok, err := svc.client.NewRequest(usernameQuery, nil).DoRaw(ctx, &resp); err != nil || !ok {
			return Namespace{}, errors.WithMessage(err, "failed to resolve namespace: no user logged in")
		}

		if resp.Data.CurrentUser.ID == "" {
			return Namespace{}, errors.New("cannot resolve current user")
		}
		return Namespace{
			ID:  resp.Data.CurrentUser.ID,
			URL: resp.Data.CurrentUser.URL,
		}, nil
	}

	var result struct {
		Data struct {
			User *struct {
				ID  string
				URL string
			}
			Organization *struct {
				ID  string
				URL string
			}
		}
		Errors []interface{}
	}
	if ok, err := svc.client.NewRequest(namespaceQuery, map[string]interface{}{
		"name": namespace,
	}).DoRaw(ctx, &result); err != nil || !ok {
		return Namespace{}, err
	}

	if result.Data.User != nil {
		return Namespace{
			ID:  result.Data.User.ID,
			URL: result.Data.User.URL,
		}, nil
	}
	if result.Data.Organization != nil {
		return Namespace{
			ID:  result.Data.Organization.ID,
			URL: result.Data.Organization.URL,
		}, nil
	}
	return Namespace{}, fmt.Errorf("failed to resolve namespace %q: no user or organization found", namespace)
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
		return nil, errors.New("no repository found: check spelling and specify the repository in the format \"<codehost_url>/owner/repo-name\" or \"repo-name\" as required by your instance")
	}
	return result.Repository, nil
}

func getGitConfig(attribute string) (string, error) {
	cmd := exec.Command("git", "config", "--get", attribute)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

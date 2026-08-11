package batches

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sourcegraph/sourcegraph/lib/batches/env"
	"github.com/sourcegraph/sourcegraph/lib/batches/overridable"
	"github.com/sourcegraph/sourcegraph/lib/batches/schema"
	"github.com/sourcegraph/sourcegraph/lib/batches/template"
	"github.com/sourcegraph/sourcegraph/lib/batches/yaml"
	"github.com/sourcegraph/sourcegraph/lib/errors"
)

// Some general notes about the struct definitions below.
//
// 1. They map _very_ closely to the batch spec JSON schema. We don't
//    auto-generate the types because we need YAML support (more on that in a
//    moment) and because no generator can currently handle oneOf fields
//    gracefully in Go, but that's a potential future enhancement.
//
// 2. Fields are tagged with _both_ JSON and YAML tags. Internally, the JSON
//    schema library needs to be able to marshal the struct to JSON for
//    validation, so we need to ensure that we're generating the right JSON to
//    represent the YAML that we unmarshalled.
//
// 3. All JSON tags include omitempty so that the schema validation can pick up
//    omitted fields. The other option here was to have everything unmarshal to
//    pointers, which is ugly and inefficient.

type BatchSpec struct {
	Version           int                      `json:"version,omitempty" yaml:"version"`
	Name              string                   `json:"name,omitempty" yaml:"name"`
	Description       string                   `json:"description,omitempty" yaml:"description"`
	On                []OnQueryOrRepository    `json:"on,omitempty" yaml:"on"`
	Workspaces        []WorkspaceConfiguration `json:"workspaces,omitempty"  yaml:"workspaces"`
	Steps             []Step                   `json:"steps,omitempty" yaml:"steps"`
	TransformChanges  *TransformChanges        `json:"transformChanges,omitempty" yaml:"transformChanges,omitempty"`
	ImportChangesets  []ImportChangeset        `json:"importChangesets,omitempty" yaml:"importChangesets"`
	ChangesetTemplate *ChangesetTemplate       `json:"changesetTemplate,omitempty" yaml:"changesetTemplate"`
	ChangesetHooks    *ChangesetHooks          `json:"changesetHooks,omitempty" yaml:"hooks,omitempty"`
}

// Hooks declares side-effect actions to run at well-defined changeset
// lifecycle events. Only allowed when Version is 3.
type ChangesetHooks struct {
	OnCIFailure     ChangesetHookAction `json:"onCIFailure" yaml:"onCIFailure,omitempty"`
	OnMergeConflict ChangesetHookAction `json:"onMergeConflict" yaml:"onMergeConflict,omitempty"`
}

// HookAction is a single action attached to a changeset lifecycle event.
//
// Hook actions reuse the Step shape from the top-level steps block.
type ChangesetHookAction struct {
	Steps []Step `json:"steps,omitempty" yaml:"steps,omitempty"`
}

type changesetHookEvent string

// Hook event names. Kept here so callers don't pass typoed strings.
const (
	ChangesetHookEventOnCIFailure     changesetHookEvent = "onCIFailure"
	ChangesetHookEventOnMergeConflict changesetHookEvent = "onMergeConflict"
)

type ChangesetTemplate struct {
	Title     string                       `json:"title,omitempty" yaml:"title"`
	Body      string                       `json:"body,omitempty" yaml:"body"`
	Branch    string                       `json:"branch,omitempty" yaml:"branch"`
	Fork      *bool                        `json:"fork,omitempty" yaml:"fork"`
	Commit    ExpandedGitCommitDescription `json:"commit" yaml:"commit"`
	Published *overridable.BoolOrString    `json:"published" yaml:"published"`
}

type GitCommitAuthor struct {
	Name  string `json:"name" yaml:"name"`
	Email string `json:"email" yaml:"email"`
}

type ExpandedGitCommitDescription struct {
	Message string           `json:"message,omitempty" yaml:"message"`
	Author  *GitCommitAuthor `json:"author,omitempty" yaml:"author"`
}

type ImportChangeset struct {
	Repository  string `json:"repository" yaml:"repository"`
	ExternalIDs []any  `json:"externalIDs" yaml:"externalIDs"`
}

type WorkspaceConfiguration struct {
	RootAtLocationOf   string `json:"rootAtLocationOf,omitempty" yaml:"rootAtLocationOf"`
	In                 string `json:"in,omitempty" yaml:"in"`
	OnlyFetchWorkspace bool   `json:"onlyFetchWorkspace,omitempty" yaml:"onlyFetchWorkspace"`
}

type OnQueryOrRepository struct {
	RepositoriesMatchingQuery string   `json:"repositoriesMatchingQuery,omitempty" yaml:"repositoriesMatchingQuery"`
	Repository                string   `json:"repository,omitempty" yaml:"repository"`
	Branch                    string   `json:"branch,omitempty" yaml:"branch"`
	Branches                  []string `json:"branches,omitempty" yaml:"branches"`
}

var ErrConflictingBranches = NewValidationError(errors.New("both branch and branches specified"))

func (oqor *OnQueryOrRepository) GetBranches() ([]string, error) {
	if oqor.Branch != "" {
		if len(oqor.Branches) > 0 {
			return nil, ErrConflictingBranches
		}
		return []string{oqor.Branch}, nil
	}
	return oqor.Branches, nil
}

type Step struct {
	Run         string            `json:"run,omitempty" yaml:"run"`
	CodingAgent *CodingAgentStep  `json:"codingAgent,omitempty" yaml:"codingAgent,omitempty"`
	BuildImage  *BuildImageStep   `json:"buildImage,omitempty" yaml:"buildImage,omitempty"`
	Container   string            `json:"container,omitempty" yaml:"container"`
	Image       string            `json:"image,omitempty" yaml:"image"`
	MaxAttempts int               `json:"maxAttempts,omitempty" yaml:"maxAttempts,omitempty"`
	Env         env.Environment   `json:"env" yaml:"env"`
	Files       map[string]string `json:"files,omitempty" yaml:"files,omitempty"`
	Outputs     Outputs           `json:"outputs,omitempty" yaml:"outputs,omitempty"`
	Mount       []Mount           `json:"mount,omitempty" yaml:"mount,omitempty"`
	If          any               `json:"if,omitempty" yaml:"if,omitempty"`
}

type CodingAgentStep struct {
	Type   string `json:"type,omitempty" yaml:"type"`
	Prompt string `json:"prompt,omitempty" yaml:"prompt"`
}

type BuildImageStep struct {
	Run       string `json:"run" yaml:"run"`
	BaseImage string `json:"baseImage" yaml:"baseImage"`
}

// MarshalJSON canonicalizes the v3 `image:` field into `container:` on the
// wire. Both fields exist on Step for ergonomic reasons (v3 specs use
// `image:`, v1/v2 specs use `container:`), but src-cli's Step has only
// `Container`. Without canonicalization, the prep-side cache key — computed
// by JSON-marshaling Step — would include `image` while the executor side
// (which round-trips through src-cli) would not, producing divergent keys
// and silent cache misses for any v3 spec. See the regression test in
// lib/batches/execution/cache.
func (s Step) MarshalJSON() ([]byte, error) {
	// Use an alias type to avoid infinite recursion through MarshalJSON.
	type stepAlias Step
	canon := stepAlias(s)
	if canon.Container == "" {
		canon.Container = canon.Image
	}
	canon.Image = ""
	return json.Marshal(canon)
}

func (s *Step) IfCondition() string {
	switch v := s.If.(type) {
	case bool:
		if v {
			return "true"
		}
		return "false"
	case string:
		return v
	default:
		return ""
	}
}

type Outputs map[string]Output

type Output struct {
	Value  string `json:"value,omitempty" yaml:"value,omitempty"`
	Format string `json:"format,omitempty" yaml:"format,omitempty"`
}

type TransformChanges struct {
	Group []Group `json:"group,omitempty" yaml:"group"`
}

type Group struct {
	Directory  string `json:"directory,omitempty" yaml:"directory"`
	Branch     string `json:"branch,omitempty" yaml:"branch"`
	Repository string `json:"repository,omitempty" yaml:"repository"`
}

type Mount struct {
	Mountpoint string `json:"mountpoint" yaml:"mountpoint"`
	Path       string `json:"path" yaml:"path"`
}

func ParseBatchSpec(data []byte) (*BatchSpec, error) {
	return parseBatchSpec(schema.BatchSpecJSON, data)
}

func parseBatchSpec(schema string, data []byte) (*BatchSpec, error) {
	var spec BatchSpec
	if err := yaml.UnmarshalValidate(schema, data, &spec); err != nil {
		var multiErr errors.MultiError
		if errors.As(err, &multiErr) {
			var newMultiError error

			for _, e := range multiErr.Errors() {
				// In case of `name` we try to make the error message more user-friendly.
				if strings.Contains(e.Error(), "name: Does not match pattern") {
					newMultiError = errors.Append(newMultiError, NewValidationError(errors.Newf("The batch change name can only contain word characters, dots and dashes. No whitespace or newlines allowed.")))
				} else {
					newMultiError = errors.Append(newMultiError, NewValidationError(e))
				}
			}

			return nil, newMultiError
		}

		return nil, err
	}

	if spec.Version == 3 {
		// Mirror v3 `image:` into `container:` so in-memory consumers that
		// read step.Container (e.g. the executor transform) keep working.
		// JSON serialization is canonicalized separately in Step.MarshalJSON
		// so prep-side cache hashing matches src-cli/executor serialization.
		for i := range spec.Steps {
			spec.Steps[i].Container = spec.Steps[i].Image
		}
	}

	var errs error
	if len(spec.Steps) != 0 && spec.ChangesetTemplate == nil {
		errs = errors.Append(errs, NewValidationError(errors.New("batch spec includes steps but no changesetTemplate")))
	}

	// v3 specs do not support changesetTemplate.published — publication is
	// driven exclusively via the batchchangeagent tools. Reject the field at
	// parse time.
	if spec.Version == 3 && spec.ChangesetTemplate != nil && spec.ChangesetTemplate.Published != nil {
		errs = errors.Append(errs, NewValidationError(errors.New("changesetTemplate.published is not supported in batch spec version 3; drive publication via the publish_changesets tool instead")))
	}

	for i, step := range spec.Steps {
		for _, mount := range step.Mount {
			if strings.Contains(mount.Path, invalidMountCharacters) {
				errs = errors.Append(errs, NewValidationError(errors.Newf("step %d mount path contains invalid characters", i+1)))
			}
			if strings.Contains(mount.Mountpoint, invalidMountCharacters) {
				errs = errors.Append(errs, NewValidationError(errors.Newf("step %d mount mountpoint contains invalid characters", i+1)))
			}
		}
		if step.CodingAgent != nil && step.Run != "" {
			errs = errors.Append(errs, NewValidationError(errors.Newf("step %d: codingAgent and run cannot be combined in the same step", i+1)))
		}
		if step.BuildImage != nil && step.Run != "" {
			errs = errors.Append(errs, NewValidationError(errors.Newf("step %d: buildImage and run cannot be combined in the same step", i+1)))
		}
		for name := range step.Files {
			if strings.Contains(name, invalidMountCharacters) {
				errs = errors.Append(errs, NewValidationError(errors.Newf("step %d files target path contains invalid characters", i+1)))
			}
		}
	}

	if hookErr := validateHooks(&spec); hookErr != nil {
		errs = errors.Append(errs, hookErr)
	}

	return &spec, errs
}

// validateHooks performs Go-level validation of spec.Hooks beyond what the
// JSON schema enforces. The schema already gates `hooks:` on `version: 3` and
// rejects unknown event names. We re-check the version invariant here so
// non-schema callers (and any future schema drift) still fail safely, and we
// run the per-step mount-character validator that the schema cannot express.
func validateHooks(spec *BatchSpec) error {
	if spec.ChangesetHooks == nil {
		return nil
	}

	var errs error

	if spec.Version != 3 {
		errs = errors.Append(errs, NewValidationError(errors.New("batch spec hooks require version: 3")))
	}

	validate := func(event changesetHookEvent, action ChangesetHookAction) {
		for i, step := range action.Steps {
			for _, mount := range step.Mount {
				if strings.Contains(mount.Path, invalidMountCharacters) {
					errs = errors.Append(errs, NewValidationError(errors.Newf(
						"hooks.%s step %d mount path contains invalid characters", event, i+1,
					)))
				}
				if strings.Contains(mount.Mountpoint, invalidMountCharacters) {
					errs = errors.Append(errs, NewValidationError(errors.Newf(
						"hooks.%s step %d mount mountpoint contains invalid characters", event, i+1,
					)))
				}
			}
		}
	}

	validate(ChangesetHookEventOnCIFailure, spec.ChangesetHooks.OnCIFailure)
	validate(ChangesetHookEventOnMergeConflict, spec.ChangesetHooks.OnMergeConflict)

	return errs
}

const invalidMountCharacters = ","

func (on *OnQueryOrRepository) String() string {
	if on.RepositoriesMatchingQuery != "" {
		return on.RepositoriesMatchingQuery
	} else if on.Repository != "" {
		return "repository:" + on.Repository
	}

	return fmt.Sprintf("%v", *on)
}

// BatchSpecValidationError is returned when parsing/using values from the batch spec failed.
type BatchSpecValidationError struct {
	err error
}

func NewValidationError(err error) BatchSpecValidationError {
	return BatchSpecValidationError{err}
}

func (e BatchSpecValidationError) Error() string {
	return e.err.Error()
}

// SkippedStepsForRepo calculates the steps required to run on the given repo.
func SkippedStepsForRepo(spec *BatchSpec, repoName string, fileMatches []string) (skipped map[int]struct{}, err error) {
	skipped = map[int]struct{}{}

	for idx, step := range spec.Steps {
		// If no if condition is set the step is always run.
		if step.IfCondition() == "" {
			continue
		}

		batchChange := template.BatchChangeAttributes{
			Name:        spec.Name,
			Description: spec.Description,
		}
		// TODO: This step ctx is incomplete, is this allowed?
		// We can at least optimize further here and do more static evaluation
		// when we have a cached result for the previous step.
		stepCtx := &template.StepContext{
			Repository: template.Repository{
				Name:        repoName,
				FileMatches: fileMatches,
			},
			BatchChange: batchChange,
		}
		static, boolVal, err := template.IsStaticBool(step.IfCondition(), stepCtx)
		if err != nil {
			return nil, err
		}

		if static && !boolVal {
			skipped[idx] = struct{}{}
		}
	}

	return skipped, nil
}

// RequiredEnvVars inspects all steps for outer environment variables used and
// compiles a deduplicated list from those.
func (s *BatchSpec) RequiredEnvVars() []string {
	requiredMap := map[string]struct{}{}
	required := []string{}
	for _, step := range s.Steps {
		for _, v := range step.Env.OuterVars() {
			if _, ok := requiredMap[v]; !ok {
				requiredMap[v] = struct{}{}
				required = append(required, v)
			}
		}
	}
	return required
}

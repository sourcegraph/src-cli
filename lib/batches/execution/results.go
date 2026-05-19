package execution

import (
	"encoding/json"

	"github.com/sourcegraph/sourcegraph/lib/batches/git"
)

// AfterStepResult is the execution result after executing a step with the given
// index in Steps.
type AfterStepResult struct {
	Version int `json:"version"`
	// Files are the changes made to Files by the step.
	ChangedFiles git.Changes `json:"changedFiles"`
	// Stdout is the output produced by the step on standard out.
	Stdout string `json:"stdout"`
	// StdoutArtifact points to externally stored standard output when Stdout is
	// too large to inline.
	StdoutArtifact *ArtifactReference `json:"stdoutArtifact,omitempty"`
	// Stderr is the output produced by the step on standard error.
	Stderr string `json:"stderr"`
	// StderrArtifact points to externally stored standard error when Stderr is
	// too large to inline.
	StderrArtifact *ArtifactReference `json:"stderrArtifact,omitempty"`
	// StepIndex is the index of the step in the list of steps.
	StepIndex int `json:"stepIndex"`
	// Diff is the cumulative `git diff` after executing the Step.
	Diff []byte `json:"diff"`
	// DiffArtifact points to an externally stored diff when Diff is too large to
	// inline.
	DiffArtifact *ArtifactReference `json:"diffArtifact,omitempty"`
	// Outputs is a copy of the Outputs after executing the Step.
	Outputs map[string]any `json:"outputs"`
	// Skipped determines whether the step was skipped.
	Skipped bool `json:"skipped"`
}

type ArtifactReference struct {
	URL              string `json:"url,omitempty"`
	ObjectStorageKey string `json:"objectStorageKey,omitempty"`
	Size             int64  `json:"size,omitempty"`
}

func (a AfterStepResult) MarshalJSON() ([]byte, error) {
	if a.Version == 2 {
		return json.Marshal(v2AfterStepResult(a))
	}
	return json.Marshal(v1AfterStepResult{
		ChangedFiles:   a.ChangedFiles,
		Stdout:         a.Stdout,
		StdoutArtifact: a.StdoutArtifact,
		Stderr:         a.Stderr,
		StderrArtifact: a.StderrArtifact,
		StepIndex:      a.StepIndex,
		Diff:           string(a.Diff),
		DiffArtifact:   a.DiffArtifact,
		Outputs:        a.Outputs,
	})
}

func (a *AfterStepResult) UnmarshalJSON(data []byte) error {
	var version versionAfterStepResult
	if err := json.Unmarshal(data, &version); err != nil {
		return err
	}
	if version.Version == 2 {
		var v2 v2AfterStepResult
		if err := json.Unmarshal(data, &v2); err != nil {
			return err
		}
		a.Version = v2.Version
		a.ChangedFiles = v2.ChangedFiles
		a.Stdout = v2.Stdout
		a.StdoutArtifact = v2.StdoutArtifact
		a.Stderr = v2.Stderr
		a.StderrArtifact = v2.StderrArtifact
		a.StepIndex = v2.StepIndex
		a.Diff = v2.Diff
		a.DiffArtifact = v2.DiffArtifact
		a.Outputs = v2.Outputs
		a.Skipped = v2.Skipped
		return nil
	}
	var v1 v1AfterStepResult
	if err := json.Unmarshal(data, &v1); err != nil {
		return err
	}
	a.ChangedFiles = v1.ChangedFiles
	a.Stdout = v1.Stdout
	a.StdoutArtifact = v1.StdoutArtifact
	a.Stderr = v1.Stderr
	a.StderrArtifact = v1.StderrArtifact
	a.StepIndex = v1.StepIndex
	a.Diff = []byte(v1.Diff)
	a.DiffArtifact = v1.DiffArtifact
	a.Outputs = v1.Outputs
	return nil
}

type versionAfterStepResult struct {
	Version int `json:"version"`
}

type v2AfterStepResult struct {
	Version        int                `json:"version"`
	ChangedFiles   git.Changes        `json:"changedFiles"`
	Stdout         string             `json:"stdout"`
	StdoutArtifact *ArtifactReference `json:"stdoutArtifact,omitempty"`
	Stderr         string             `json:"stderr"`
	StderrArtifact *ArtifactReference `json:"stderrArtifact,omitempty"`
	StepIndex      int                `json:"stepIndex"`
	Diff           []byte             `json:"diff"`
	DiffArtifact   *ArtifactReference `json:"diffArtifact,omitempty"`
	Outputs        map[string]any     `json:"outputs"`
	Skipped        bool               `json:"skipped"`
}

type v1AfterStepResult struct {
	ChangedFiles   git.Changes        `json:"changedFiles"`
	Stdout         string             `json:"stdout"`
	StdoutArtifact *ArtifactReference `json:"stdoutArtifact,omitempty"`
	Stderr         string             `json:"stderr"`
	StderrArtifact *ArtifactReference `json:"stderrArtifact,omitempty"`
	StepIndex      int                `json:"stepIndex"`
	Diff           string             `json:"diff"`
	DiffArtifact   *ArtifactReference `json:"diffArtifact,omitempty"`
	Outputs        map[string]any     `json:"outputs"`
}

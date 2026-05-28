// Package types holds the codingagent contract shared with individual
// coding-agent implementations; split out to avoid an import cycle.
package types

import (
	batcheslib "github.com/sourcegraph/sourcegraph/lib/batches"
)

const (
	ModelProviderTokenEnvVar = "SRC_BATCHES_MODEL_PROVIDER_TOKEN"
	JobIDEnvVar              = "SRC_BATCHES_JOB_ID"
	JobIDHeaderName          = "X-Sourcegraph-Job-ID"
	InstallDir               = "/tmp/sg-codingagent/bin"
)

type Agent interface {
	Type() batcheslib.CodingAgentType
	// RunCommand returns the shell command for the agent. The rendered
	// prompt MUST be shell-quoted.
	RunCommand(renderedPrompt, modelProviderURL string) string
	// ImageRequirements lists binaries that must be on PATH in the run
	// container before InstallScript runs.
	ImageRequirements() []string
	// InstallScript installs the agent at a pinned version into InstallDir.
	InstallScript() string
}

// Package types holds the contract shared between the codingagent registry
// and individual coding-agent implementations, in a leaf package to avoid an
// import cycle with the registry.
package types

import (
	batcheslib "github.com/sourcegraph/sourcegraph/lib/batches"
)

const ModelProviderTokenEnvVar = "SRC_BATCHES_MODEL_PROVIDER_TOKEN"

// JobIDEnvVar binds ModelProviderTokenEnvVar to its job; sent as JobIDHeaderName.
const JobIDEnvVar = "SRC_BATCHES_JOB_ID"

const JobIDHeaderName = "X-Sourcegraph-Job-ID"

// InstallDir is where each Agent installs its pinned binary; invoked by
// absolute path so we ignore any copy shipped by the image.
const InstallDir = "/tmp/sg-codingagent/bin"

type Agent interface {
	Type() batcheslib.CodingAgentType
	// RunCommand receives the already-templated prompt and MUST shell-quote it.
	RunCommand(renderedPrompt, modelProviderURL string) string
	// ModelProviderRoutes returns routes whose WirePath is relative to the
	// agent type; the registry prefixes "/<agent-type>" before exposing them.
	ModelProviderRoutes() []ModelProviderRoute
	// ImageRequirements lists binaries that must be on PATH in the run
	// container (e.g. "curl" so InstallScript can fetch the agent). The
	// registry emits a `command -v` check for each before InstallScript.
	ImageRequirements() []string
	// InstallScript returns shell that installs the agent at a pinned
	// version into a Sourcegraph-owned scratch dir and prepends it to PATH.
	InstallScript() string
}

// ModelProviderRoute is one wire→upstream mapping served by the Sourcegraph
// model-provider proxy (mounted under /.executors/model-provider/batches).
type ModelProviderRoute struct {
	// WirePath is the proxy subpath the agent CLI POSTs to, relative to the
	// agent type (e.g. "/responses"). The registry prefixes "/<agent-type>"
	// before registering it on the router (e.g. "/codex/responses").
	WirePath string
	// UpstreamPath is the path on the Sourcegraph Model Provider upstream
	// (e.g. Cody Gateway) that serves WirePath, such as
	// "/v1/completions/openai-responses".
	UpstreamPath string
}

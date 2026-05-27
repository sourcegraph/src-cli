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

type Agent interface {
	Type() batcheslib.CodingAgentType
	// RunCommand receives the already-templated prompt and MUST shell-quote it.
	RunCommand(renderedPrompt, modelProviderURL string) string
	// ModelProviderRoutes returns routes whose WirePath is relative to the
	// agent type; the registry prefixes "/<agent-type>" before exposing them.
	ModelProviderRoutes() []ModelProviderRoute
	// ImageRequirements returns binaries the agent expects on PATH in the run
	// container. The registry emits a check before RunCommand.
	ImageRequirements() []string
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

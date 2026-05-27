// Package codex implements the codex coding agent run-command rewrite.
package codex

import (
	"fmt"

	"github.com/kballard/go-shellquote"

	batcheslib "github.com/sourcegraph/sourcegraph/lib/batches"
	"github.com/sourcegraph/sourcegraph/lib/batches/codingagent/types"
)

const model = "gpt-5.4"

var routes = []types.ModelProviderRoute{
	{WirePath: "/responses", UpstreamPath: "/v1/completions/openai-responses"},
}

type Agent struct{}

func (Agent) Type() batcheslib.CodingAgentType                { return batcheslib.CodingAgentTypeCodex }
func (Agent) ModelProviderRoutes() []types.ModelProviderRoute { return routes }
func (Agent) ImageRequirements() []string                     { return []string{"codex"} }

func (Agent) RunCommand(prompt, modelProviderURL string) string {
	return shellquote.Join(
		"codex",
		"exec",
		"--json",
		"--sandbox", "danger-full-access",
		"--ephemeral",
		"--model", model,
		"-c", `approval_policy="never"`,
		"-c", `model_reasoning_effort="medium"`,
		"-c", `model_provider="sourcegraph"`,
		"-c", `model_providers.sourcegraph.name="Sourcegraph"`,
		"-c", fmt.Sprintf(`model_providers.sourcegraph.base_url=%q`, modelProviderURL),
		"-c", fmt.Sprintf(`model_providers.sourcegraph.env_key=%q`, types.ModelProviderTokenEnvVar),
		"-c", fmt.Sprintf(`model_providers.sourcegraph.env_http_headers={%q=%q}`, types.JobIDHeaderName, types.JobIDEnvVar),
		"-c", `model_providers.sourcegraph.wire_api="responses"`,
		prompt,
	)
}

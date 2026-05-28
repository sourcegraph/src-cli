// Package codingagent rewrites v3 batch spec coding-agent steps into the
// shell commands that drive them. Register new agents in the agents list.
package codingagent

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/kballard/go-shellquote"

	batcheslib "github.com/sourcegraph/sourcegraph/lib/batches"
	"github.com/sourcegraph/sourcegraph/lib/batches/codex"
	"github.com/sourcegraph/sourcegraph/lib/batches/codingagent/types"
	"github.com/sourcegraph/sourcegraph/lib/batches/template"
	"github.com/sourcegraph/sourcegraph/lib/errors"
)

// ErrUnknownType is returned when a codingAgent step references a type that
// has no registered Agent.
var ErrUnknownType = errors.New("unknown codingAgent type")

// RenderRunCommand returns the shell-quoted command that runs agentStep. The
// prompt is rendered before quoting; reversing would let templated values
// break out of the shell quoting.
func RenderRunCommand(agentStep *batcheslib.CodingAgentStep, modelProviderURL string, stepCtx *template.StepContext) (string, error) {
	a, ok := agents[agentStep.Type]
	if !ok {
		return "", errors.Wrapf(ErrUnknownType, "codingAgent type %q", agentStep.Type)
	}
	var renderedPrompt bytes.Buffer
	if err := template.RenderStepTemplate("codingagent-prompt", agentStep.Prompt, &renderedPrompt, stepCtx); err != nil {
		return "", errors.Wrap(err, "rendering codingAgent.prompt")
	}
	prefixed := strings.TrimRight(modelProviderURL, "/") + "/" + string(a.Type())

	var b strings.Builder
	for _, binary := range a.ImageRequirements() {
		b.WriteString(failIfMissing(a.Type(), binary))
	}
	b.WriteString(a.InstallScript())
	b.WriteString(a.RunCommand(renderedPrompt.String(), prefixed))
	return b.String(), nil
}

// failIfMissing returns a shell snippet that, when prepended to the step
// script, writes a message to stderr and exits the container with status 1
// if binary isn't on PATH.
func failIfMissing(agentType batcheslib.CodingAgentType, binary string) string {
	msg := fmt.Sprintf(
		"codingAgent %q requires %q on PATH in the run container",
		agentType, binary,
	)
	return fmt.Sprintf("command -v %s >/dev/null 2>&1 || { echo %s >&2; exit 1; }\n",
		binary,
		shellquote.Join(msg),
	)
}

var agents = func() map[batcheslib.CodingAgentType]types.Agent {
	out := map[batcheslib.CodingAgentType]types.Agent{}
	for _, a := range []types.Agent{
		codex.Agent{},
	} {
		if _, exists := out[a.Type()]; exists {
			panic("duplicate codingagent agent for " + a.Type())
		}
		out[a.Type()] = a
	}
	return out
}()

// RegisteredModelProviderRoutes returns the model-provider proxy routes
// contributed by every registered Agent, each WirePath prefixed with
// its agent type.
func RegisteredModelProviderRoutes() []types.ModelProviderRoute {
	var out []types.ModelProviderRoute
	for _, a := range agents {
		prefix := "/" + string(a.Type())
		for _, route := range a.ModelProviderRoutes() {
			out = append(out, types.ModelProviderRoute{
				WirePath:     prefix + route.WirePath,
				UpstreamPath: route.UpstreamPath,
			})
		}
	}
	return out
}

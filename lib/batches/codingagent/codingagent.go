// Package codingagent rewrites v3 batch spec coding-agent steps into shell
// commands. Register new agents in the agents list.
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

var ErrUnknownType = errors.New("unknown codingAgent type")

// RenderRunCommand returns the shell command that runs agentStep.
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

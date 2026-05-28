// Package codex implements the codex coding agent run-command rewrite.
package codex

import (
	"fmt"

	"github.com/kballard/go-shellquote"

	batcheslib "github.com/sourcegraph/sourcegraph/lib/batches"
	"github.com/sourcegraph/sourcegraph/lib/batches/codingagent/types"
)

const model = "gpt-5.4"

// pinnedVersion is the codex CLI release we test against. Bump in lockstep
// with the model_providers.* config keys below; codex CLI is pre-v1.
const pinnedVersion = "0.134.0"

var routes = []types.ModelProviderRoute{
	{WirePath: "/responses", UpstreamPath: "/v1/completions/openai-responses"},
}

type Agent struct{}

func (Agent) Type() batcheslib.CodingAgentType                { return batcheslib.CodingAgentTypeCodex }
func (Agent) ModelProviderRoutes() []types.ModelProviderRoute { return routes }
func (Agent) ImageRequirements() []string                     { return []string{"curl", "tar"} }

// InstallScript installs codex from GitHub Releases into types.InstallDir.
func (Agent) InstallScript() string {
	return fmt.Sprintf(`_install_dir=%s
_version=%s
_codex_arch=$(uname -m)
case "$_codex_arch" in
  x86_64)  _codex_triple=x86_64-unknown-linux-musl ;;
  aarch64) _codex_triple=aarch64-unknown-linux-musl ;;
  *) echo "codingAgent codex: unsupported architecture: $_codex_arch" >&2; exit 1 ;;
esac
mkdir -p "$_install_dir"
curl -fsSL "https://github.com/openai/codex/releases/download/rust-v${_version}/codex-${_codex_triple}.tar.gz" | tar -xz -C "$_install_dir"
mv "$_install_dir/codex-${_codex_triple}" "$_install_dir/codex"
chmod +x "$_install_dir/codex"
"$_install_dir/codex" --version >/dev/null || { echo "codingAgent codex: install verification failed (cannot exec $_install_dir/codex)" >&2; exit 1; }
`, types.InstallDir, pinnedVersion)
}

func (Agent) RunCommand(prompt, modelProviderURL string) string {
	return shellquote.Join(
		types.InstallDir+"/codex",
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

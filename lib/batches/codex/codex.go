// Package codex implements the codex coding agent.
package codex

import (
	"fmt"

	"github.com/kballard/go-shellquote"

	batcheslib "github.com/sourcegraph/sourcegraph/lib/batches"
	"github.com/sourcegraph/sourcegraph/lib/batches/codingagent/types"
)

const (
	model         = "gpt-5.4"
	pinnedVersion = "0.134.0"
)

type Agent struct{}

func (Agent) Type() batcheslib.CodingAgentType { return batcheslib.CodingAgentTypeCodex }
func (Agent) ImageRequirements() []string      { return []string{"curl", "tar"} }

func (Agent) InstallScript() string {
	return fmt.Sprintf(`_install_dir=%s
_version=%s

case "$(uname -m)" in
  x86_64)  _triple=x86_64-unknown-linux-musl ;;
  aarch64) _triple=aarch64-unknown-linux-musl ;;
  *) echo "codingAgent codex: unsupported architecture $(uname -m)" >&2; exit 1 ;;
esac

# Stage in a temp dir and mv into place so a failed retry can't leave a half-written binary behind.
_url="https://github.com/openai/codex/releases/download/rust-v${_version}/codex-${_triple}.tar.gz"
_tmp=$(mktemp -d "${TMPDIR:-/tmp}/sg-codex.XXXXXX")
mkdir -p "$_install_dir"
curl -fsSL "$_url" | tar -xz -C "$_tmp" || { echo "codingAgent codex: download/extract failed: $_url" >&2; exit 1; }
chmod +x "$_tmp/codex-${_triple}"
mv -f "$_tmp/codex-${_triple}" "$_install_dir/codex"
rm -rf "$_tmp"

_actual=$("$_install_dir/codex" --version 2>&1) || { echo "codingAgent codex: cannot exec $_install_dir/codex: $_actual" >&2; exit 1; }
case "$_actual" in
  *"$_version"*) ;;
  *) echo "codingAgent codex: version mismatch: want $_version, got: $_actual" >&2; exit 1 ;;
esac
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
		prompt,
	)
}

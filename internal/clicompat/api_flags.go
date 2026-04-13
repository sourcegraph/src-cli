package clicompat

import (
	"os"

	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/urfave/cli/v3"
)

// WithAPIFlags appends the standard API-related flags used by legacy src
func WithAPIFlags(baseFlags ...cli.Flag) []cli.Flag {
	var flagTable = []struct {
		name  string
		value bool
		text  string
	}{
		{"dump-requests", false, "Log GraphQL requests and responses to stdout"},
		{"get-curl", false, "Print the curl command for executing this query and exit (WARNING: includes printing your access token!)"},
		{"trace", false, "Log the trace ID for requests. See https://docs.sourcegraph.com/admin/observability/tracing"},
		{"insecure-skip-verify", false, "Skip validation of TLS certificates against trusted chains"},
		{"user-agent-telemetry", defaultAPIUserAgentTelemetry(), "Include the operating system and architecture in the User-Agent sent with requests to Sourcegraph"},
	}

	flags := append([]cli.Flag{}, baseFlags...)
	for _, item := range flagTable {
		flags = append(flags, &cli.BoolFlag{
			Name:  item.name,
			Value: item.value,
			Usage: item.text,
		})
	}

	return flags
}

// APIFlagsFromCmd reads the shared API-related flags from a command into api.Flags
func APIFlagsFromCmd(cmd *cli.Command) *api.Flags {
	return api.NewFlagsFromValues(
		cmd.Bool("dump-requests"),
		cmd.Bool("get-curl"),
		cmd.Bool("trace"),
		cmd.Bool("insecure-skip-verify"),
		cmd.Bool("user-agent-telemetry"),
	)
}

func defaultAPIUserAgentTelemetry() bool {
	return os.Getenv("SRC_DISABLE_USER_AGENT_TELEMETRY") == ""
}

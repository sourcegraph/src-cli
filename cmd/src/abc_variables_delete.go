package main

import (
	"context"
	"fmt"

	"github.com/sourcegraph/src-cli/internal/clicompat"
	"github.com/sourcegraph/src-cli/internal/cmderrors"
	"github.com/urfave/cli/v3"
)

var abcVariablesDeleteCommand = clicompat.Wrap(&cli.Command{
	Name:  "delete",
	Usage: "delete workflow instance variables",
	Description: `Usage:

	src abc variables delete [command options] <workflow-instance-id> [<name> ...]

Examples:

  Delete a variable from a workflow instance:

	$ src abc variables delete QWdlbnRpY1dvcmtmbG93SW5zdGFuY2U6MQ== approval

  Delete multiple variables in one request:

	$ src abc variables delete QWdlbnRpY1dvcmtmbG93SW5zdGFuY2U6MQ== --var approval --var checkpoints`,
	DisableSliceFlagSeparator: true,
	Flags: clicompat.WithAPIFlags(
		&cli.StringSliceFlag{
			Name:  "var",
			Usage: "Variable name to delete. Repeat for multiple names.",
		},
	),
	Action: func(ctx context.Context, c *cli.Command) error {
		if c.NArg() == 0 {
			return cmderrors.Usage("must provide a workflow instance ID")
		}

		instanceID := c.Args().First()
		variableNames, err := parseABCVariableNames(c.Args().Tail(), abcVariableArgs(c.StringSlice("var")))
		if err != nil {
			return err
		}

		variables := make([]map[string]string, 0, len(variableNames))
		for _, key := range variableNames {
			variables = append(variables, map[string]string{
				"key":   key,
				"value": "null",
			})
		}

		apiFlags := clicompat.APIFlagsFromCmd(c)
		client := cfg.apiClient(apiFlags, c.Writer)
		if err := updateABCWorkflowInstanceVariables(ctx, client, instanceID, variables); err != nil {
			return err
		}

		if apiFlags.GetCurl() {
			return nil
		}

		fmt.Fprintf(c.Writer, "Removed variables %q from workflow instance %q.\n", variableNames, instanceID)
		return nil
	},
})

func parseABCVariableNames(positional []string, flagged abcVariableArgs) ([]string, error) {
	variableNames := append([]string{}, positional...)
	variableNames = append(variableNames, flagged...)
	if len(variableNames) == 0 {
		return nil, cmderrors.Usage("must provide at least one variable name")
	}

	for _, name := range variableNames {
		if name == "" {
			return nil, cmderrors.Usage("variable names must not be empty")
		}
	}

	return variableNames, nil
}

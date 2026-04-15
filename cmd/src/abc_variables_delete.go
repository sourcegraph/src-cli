package main

import (
	"context"
	"fmt"
	"io"
	"slices"

	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/clicompat"
	"github.com/sourcegraph/src-cli/internal/cmderrors"
	"github.com/urfave/cli/v3"
)

var abcVariablesDeleteCommand = clicompat.Wrap(&cli.Command{
	Name:      "delete",
	Usage:     "Delete variables on a workflow instance",
	UsageText: "src abc variables delete [options] <workflow-instance-id> [<name> ...]",
	Description: `
Delete workflow instance variables

Examples:

  Delete a variable from a workflow instance:

	    $ src abc variables delete QWdlbnRpY1dvcmtmbG93SW5zdGFuY2U6MQ== approval

  Delete multiple variables in one request:

	    $ src abc variables delete QWdlbnRpY1dvcmtmbG93SW5zdGFuY2U6MQ== --var approval --var checkpoints
`,
	Flags: clicompat.WithAPIFlags(
		&cli.StringSliceFlag{
			Name:  "var",
			Usage: "Variable name to delete. Repeat for multiple names.",
		},
	),
	Action: func(ctx context.Context, cmd *cli.Command) error {
		if !cmd.Args().Present() {
			return cmderrors.Usage("must provide a workflow instance ID")
		}

		instanceID := cmd.Args().First()
		client := cfg.apiClient(clicompat.APIFlagsFromCmd(cmd), cmd.Writer)
		varArgs := abcVariableArgs(cmd.StringSlice("var"))

		if len(varArgs) == 0 {
			return cmderrors.Usage("must provide at least one variable name")
		}
		return runABCVariablesDelete(ctx, client, instanceID, cmd.Args().Tail(), varArgs, cmd.Writer, cmd.Bool("get-curl"))
	},
})

func parseABCVariableNames(positional []string, flagged abcVariableArgs) ([]string, error) {
	variableNames := append([]string{}, positional...)
	variableNames = append(variableNames, flagged...)

	if slices.Contains(variableNames, "") {
		return nil, cmderrors.Usage("variable names must not be empty")
	}

	return variableNames, nil
}

func runABCVariablesDelete(ctx context.Context, client api.Client, instanceID string, positional []string, flagged abcVariableArgs, output io.Writer, getCurl bool) error {
	variableNames, err := parseABCVariableNames(positional, flagged)
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

	if err := updateABCWorkflowInstanceVariables(ctx, client, instanceID, variables); err != nil {
		return err
	}

	if getCurl {
		return nil
	}

	_, err = fmt.Fprintf(output, "Removed variables %q from workflow instance %q.\n", variableNames, instanceID)
	return err
}

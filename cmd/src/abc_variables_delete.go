package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/cmderrors"
)

func init() {
	usage := `
Examples:

  Delete a variable from a workflow instance:

	    $ src abc variables delete QWdlbnRpY1dvcmtmbG93SW5zdGFuY2U6MQ== approval

  Delete multiple variables in one request:

	    $ src abc variables delete QWdlbnRpY1dvcmtmbG93SW5zdGFuY2U6MQ== --var approval --var checkpoints
	`

	flagSet := flag.NewFlagSet("delete", flag.ExitOnError)
	var variableArgs abcVariableArgs
	flagSet.Var(&variableArgs, "var", "Variable name to delete. Repeat for multiple names.")
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src abc variables %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}
	apiFlags := api.NewFlags(flagSet)

	handler := func(args []string) error {
		if len(args) == 0 {
			return cmderrors.Usage("must provide a workflow instance ID")
		}

		instanceID := args[0]
		variableArgs = nil
		if err := flagSet.Parse(args[1:]); err != nil {
			return err
		}

		variableNames, err := parseABCVariableNames(flagSet.Args(), variableArgs)
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

		client := cfg.apiClient(apiFlags, flagSet.Output())
		if err := updateABCWorkflowInstanceVariables(context.Background(), client, instanceID, variables); err != nil {
			return err
		}

		if apiFlags.GetCurl() {
			return nil
		}

		fmt.Printf("Removed variables %q from workflow instance %q.\n", variableNames, instanceID)
		return nil
	}

	abcVariablesCommands = append(abcVariablesCommands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}

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

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

	    $ src abc QWdlbnRpY1dvcmtmbG93SW5zdGFuY2U6MQ== variables delete approval
	`

	flagSet := flag.NewFlagSet("delete", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src abc <instance-id> variables %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}
	apiFlags := api.NewFlags(flagSet)

	handler := func(args []string) error {
		if len(args) == 0 {
			return cmderrors.Usage("must provide an instance ID")
		}

		instanceID := args[0]
		if err := flagSet.Parse(args[1:]); err != nil {
			return err
		}
		if flagSet.NArg() != 1 {
			return cmderrors.Usage("must provide exactly one variable name")
		}

		key := flagSet.Arg(0)
		client := cfg.apiClient(apiFlags, flagSet.Output())
		if err := updateABCWorkflowInstanceVariables(context.Background(), client, instanceID, []map[string]string{{
			"key":   key,
			"value": "null",
		}}); err != nil {
			return err
		}

		if apiFlags.GetCurl() {
			return nil
		}

		fmt.Printf("Removed variable %q from workflow instance %q.\n", key, instanceID)
		return nil
	}

	abcVariablesCommands = append(abcVariablesCommands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}

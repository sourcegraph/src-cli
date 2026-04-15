package main

import (
	"flag"
	"fmt"

	"github.com/sourcegraph/src-cli/internal/cmderrors"
)

var abcVariablesCommands commander

func init() {
	usage := `'src abc <workflow-instance-id> variables' is a tool that manages workflow instance variables on a Sourcegraph instance.

Usage:

	src abc <workflow-instance-id> variables command [command options]

The commands are:

	set	set workflow instance variables
	delete	delete a workflow instance variable

Use "src abc <workflow-instance-id> variables [command] -h" for more information about a command.
`

	flagSet := flag.NewFlagSet("variables", flag.ExitOnError)
	usageFunc := func() {
		fmt.Println(usage)
	}
	flagSet.Usage = usageFunc
	handler := func(args []string) error {
		if len(args) == 0 {
			return cmderrors.Usage("must provide a workflow instance ID")
		}

		instanceID := args[0]
		if err := flagSet.Parse(args[1:]); err != nil {
			return err
		}

		if flagSet.NArg() == 0 || flagSet.Arg(0) == "help" {
			flagSet.SetOutput(flag.CommandLine.Output())
			flagSet.Usage()
			return nil
		}

		abcVariablesCommands.runWithPrefixArgs("src abc <workflow-instance-id> variables", []string{instanceID}, flagSet.Args())
		return nil
	}

	abcCommands = append(abcCommands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}

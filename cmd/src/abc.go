package main

import (
	"flag"
	"fmt"

	"github.com/sourcegraph/src-cli/internal/cmderrors"
)

var abcCommands commander

func init() {
	usage := `'src abc' is a tool that manages agentic batch changes on a Sourcegraph instance.

Usage:

	src abc <workflow-instance-id> command [command options]

The commands are:

	variables	manage workflow instance variables

Use "src abc <workflow-instance-id> [command] -h" for more information about a command.
`

	flagSet := flag.NewFlagSet("abc", flag.ExitOnError)
	usageFunc := func() {
		fmt.Println(usage)
	}
	flagSet.Usage = usageFunc
	handler := func(args []string) error {
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		if flagSet.NArg() == 0 || flagSet.Arg(0) == "help" {
			flagSet.SetOutput(flag.CommandLine.Output())
			flagSet.Usage()
			return nil
		}

		if flagSet.NArg() < 2 {
			return cmderrors.Usage("must provide a workflow instance ID and subcommand")
		}

		instanceID := flagSet.Arg(0)
		abcCommands.runWithPrefixArgs("src abc <workflow-instance-id>", []string{instanceID}, flagSet.Args()[1:])
		return nil
	}

	commands = append(commands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}

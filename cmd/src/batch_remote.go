package main

import (
	"flag"
	"fmt"
)

var batchRemoteCommands commander

func init() {
	usage := `'src batch remote' manages server side batch changes.

Usage:

    src batch remote command [command options]

The commands are:

    run     runs a batch spec on the Sourcegraph instance

Use "src batch [command] -h" for more information about a command.

`

	flagSet := flag.NewFlagSet("remote", flag.ExitOnError)
	handler := func(args []string) error {
		batchRemoteCommands.run(flagSet, "src batch remote", usage, args)
		return nil
	}

	batchCommands = append(batchCommands, &command{
		flagSet:   flagSet,
		aliases:   []string{"server", "ssbc"},
		handler:   handler,
		usageFunc: func() { fmt.Println(usage) },
	})
}

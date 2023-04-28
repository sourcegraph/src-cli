package main

import (
	"flag"
	"fmt"
)

var scoutCommands commander

func init() {
    usage := `'src scout' is a tool that provides monitoring for Sourcegraph resource allocations

    EXPERIMENTAL: 'scout' is an experimental command in the 'src' tool

    Usage: 
        
        src scout command [command options]

    The commands are:
        
        resources       print all known sourcegraph resources
        estimate        reccommend resource allocation for one or many services
        usage           get CPU, memory and current disk usage
        watch           track resource usage in real time

    Use "src scout [command] -h" for more information about a command.
    `

    flagSet := flag.NewFlagSet("scout", flag.ExitOnError)
    handler := func(args []string) error {
        scoutCommands.run(flagSet, "src scout", usage, args)
        return nil
    }

    commands = append(commands, &command{
        flagSet: flagSet,
        aliases: []string{"scout"},
        handler: handler,
        usageFunc: func() {
            fmt.Println(usage)
        },
    })
}

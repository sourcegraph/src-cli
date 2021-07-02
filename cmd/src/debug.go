package main

import (
	"flag"
	"fmt"

	"github.com/sourcegraph/src-cli/internal/exec"
)

func init() {
	flagSet := flag.NewFlagSet("debug", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), `'src debug' gathers and bundles debug data from a Sourcegraph deployment.

USAGE
  src [-v] debug
`)
	}

	handler := func(args []string) error {
		err := flagSet.Parse(args)
		if err != nil {
			return err
		}
		cmd := exec.Command("kubectl", "get", "events")
		stdout, err := cmd.Output()
		if err != nil {
			return err
		}
		fmt.Print(string(stdout))
		return nil
	}

	// Register the command.
	commands = append(commands, &command{
		aliases:   []string{"debug-dump"},
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}

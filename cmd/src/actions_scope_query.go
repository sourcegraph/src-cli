package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/pkg/errors"
)

func init() {
	usage := `
List the repositories that are matched by the "scopeQuery" in an action definition. This command is meant to help with creating action definitions to be used with 'src actions exec'.

Examples:

  List the names of the repositories that are returned by the "scopeQuery" in ~/action.json:

		$ src actions scope-query -f ~/run-gofmt-in-dockerfile.json

`

	flagSet := flag.NewFlagSet("scope-query", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src actions %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}

	var (
		fileFlag               = flagSet.String("f", "-", "The action file. If not given or '-' standard input is used. (Required)")
		includeUnsupportedFlag = flagSet.Bool("include-unsupported", false, "When specified, also repos from unsupported codehosts are processed. Those can be created once the integration is done.")
	)

	handler := func(args []string) error {
		err := flagSet.Parse(args)
		if err != nil {
			return err
		}

		var actionFile []byte

		if *fileFlag == "-" {
			actionFile, err = ioutil.ReadAll(os.Stdin)
		} else {
			actionFile, err = ioutil.ReadFile(*fileFlag)
		}
		if err != nil {
			return err
		}

		var action Action
		if err := jsonxUnmarshal(string(actionFile), &action); err != nil {
			return errors.Wrap(err, "invalid JSON action file")
		}

		ctx := context.Background()

		if *verbose {
			log.Printf("# scopeQuery in action definition: %s\n", action.ScopeQuery)

			if *includeUnsupportedFlag {
				log.Printf("# Including repositories on unsupported codehost.\n")
			}
		}

		logger := newActionLogger(*verbose, false)
		repos, err := actionRepos(ctx, action.ScopeQuery, *includeUnsupportedFlag, logger)
		if err != nil {
			return err
		}
		for _, repo := range repos {
			fmt.Println(repo.Name)
		}

		return nil
	}

	// Register the command.
	actionsCommands = append(actionsCommands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}

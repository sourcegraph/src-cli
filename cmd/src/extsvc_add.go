package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/sourcegraph/sourcegraph/lib/errors"
	"github.com/sourcegraph/src-cli/internal/api"
)

func init() {
	usage := `
  Examples:

  Add an external service configuration on the Sourcegraph instance:

  $ cat new-config.json | src extsvc add
  $ src extsvc add -name 'My GitHub connection' new-config.json
  `

	flagSet := flag.NewFlagSet("add", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src extsvc %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}
	var (
		nameFlag = flagSet.String("name", "", "exact name of the external service to add")
		kindFlag = flagSet.String("kind", "", "kind of the external service to add")
		apiFlags = api.NewFlags(flagSet)
	)

	handler := func(args []string) (err error) {
		ctx := context.Background()
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		if *nameFlag == "" {
			return errors.New("-name must be provided")
		}

		client := cfg.apiClient(apiFlags, flagSet.Output())
		if *nameFlag != "" {
			_, err := lookupExternalService(ctx, client, "", *nameFlag)
			if err != errServiceNotFound {
				return errors.New("service already exists")
			}
		}

		var addJSON []byte
		if len(flagSet.Args()) == 1 {
			addJSON, err = os.ReadFile(flagSet.Arg(0))
			if err != nil {
				return err
			}
		}
		if !isatty.IsTerminal(os.Stdin.Fd()) {
			// stdin is a pipe not a terminal
			addJSON, err = io.ReadAll(os.Stdin)
			if err != nil {
				return err
			}
		}

		addExternalServiceInput := map[string]interface{}{
			"kind":        strings.ToUpper(*kindFlag),
			"displayName": *nameFlag,
			"config":      string(addJSON),
		}

		queryVars := map[string]interface{}{
			"input": addExternalServiceInput,
		}

		var result struct{} // TODO: future: allow formatting resulting external service
		if ok, err := client.NewRequest(externalServicesAddMutation, queryVars).Do(ctx, &result); err != nil {
			if strings.Contains(err.Error(), "Additional property exclude is not allowed") {
				return errors.New(`specified external service does not support repository "exclude" list`)
			}
			return err
		} else if ok {
			fmt.Println("External service created:", *nameFlag)
		}
		return nil
	}

	// Register the command.
	extsvcCommands = append(extsvcCommands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}

const externalServicesAddMutation = `
  mutation AddExternalService($input: AddExternalServiceInput!) {
    addExternalService(input: $input) {
      id
      warning
    }
  }`

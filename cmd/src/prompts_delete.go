package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/sourcegraph/sourcegraph/lib/errors"

	"github.com/sourcegraph/src-cli/internal/api"
)

func init() {
	usage := `
Examples:

  Delete a prompt by ID:

    	$ src prompts delete -id=<prompt-id>

`

	flagSet := flag.NewFlagSet("delete", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src prompts %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}
	var (
		idFlag   = flagSet.String("id", "", "The ID of the prompt to delete")
		apiFlags = api.NewFlags(flagSet)
	)

	handler := func(args []string) error {
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		if *idFlag == "" {
			return errors.New("provide the ID of the prompt to delete")
		}

		client := cfg.apiClient(apiFlags, flagSet.Output())

		query := `mutation DeletePrompt($id: ID!) {
	deletePrompt(id: $id) {
		alwaysNil
	}
}
`

		var result struct {
			DeletePrompt struct {
				AlwaysNil interface{} `json:"alwaysNil"`
			}
		}

		if ok, err := client.NewRequest(query, map[string]interface{}{
			"id": *idFlag,
		}).Do(context.Background(), &result); err != nil || !ok {
			return err
		}

		fmt.Println("Prompt deleted successfully.")
		return nil
	}

	// Register the command.
	promptsCommands = append(promptsCommands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}

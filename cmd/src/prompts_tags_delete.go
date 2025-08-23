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

  Delete a prompt tag by ID:

    	$ src prompts tags delete <tag-id>

`

	flagSet := flag.NewFlagSet("delete", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src prompts tags %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}
	var (
		apiFlags = api.NewFlags(flagSet)
	)

	handler := func(args []string) error {
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		// Check for tag ID as positional argument
		if len(flagSet.Args()) != 1 {
			return errors.New("provide exactly one tag ID as an argument")
		}
		tagID := flagSet.Arg(0)

		client := cfg.apiClient(apiFlags, flagSet.Output())

		query := `mutation DeletePromptTag($id: ID!) {
	deletePromptTag(id: $id) {
		alwaysNil
	}
}
`

		var result struct {
			DeletePromptTag struct {
				AlwaysNil interface{} `json:"alwaysNil"`
			}
		}

		if ok, err := client.NewRequest(query, map[string]interface{}{
			"id": tagID,
		}).Do(context.Background(), &result); err != nil || !ok {
			return err
		}

		fmt.Println("Prompt tag deleted successfully.")
		return nil
	}

	// Register the command.
	promptsTagsCommands = append(promptsTagsCommands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}

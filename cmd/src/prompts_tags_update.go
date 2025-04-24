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

  Update a prompt tag:

    	$ src prompts tags update -id=<tag-id> -name="updated-tag-name"

`

	flagSet := flag.NewFlagSet("update", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src prompts tags %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}
	var (
		idFlag   = flagSet.String("id", "", "The ID of the tag to update")
		nameFlag = flagSet.String("name", "", "The new name for the tag")
		apiFlags = api.NewFlags(flagSet)
	)

	handler := func(args []string) error {
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		if *idFlag == "" {
			return errors.New("provide the ID of the tag to update")
		}

		if *nameFlag == "" {
			return errors.New("provide a new name for the tag")
		}

		client := cfg.apiClient(apiFlags, flagSet.Output())

		query := `mutation UpdatePromptTag($id: ID!, $input: PromptTagUpdateInput!) {
	updatePromptTag(id: $id, input: $input) {
		...PromptTagFields
	}
}
` + promptTagFragment

		var result struct {
			UpdatePromptTag PromptTag
		}
		if ok, err := client.NewRequest(query, map[string]interface{}{
			"id": *idFlag,
			"input": map[string]interface{}{
				"name": *nameFlag,
			},
		}).Do(context.Background(), &result); err != nil || !ok {
			return err
		}

		fmt.Printf("Prompt tag updated: %s\n", result.UpdatePromptTag.ID)
		return nil
	}

	// Register the command.
	promptsTagsCommands = append(promptsTagsCommands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}

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

    	$ src prompts tags update <tag-id> -name="updated-tag-name"

`

	flagSet := flag.NewFlagSet("update", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src prompts tags %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}
	var (
		nameFlag = flagSet.String("name", "", "The new name for the tag")
		apiFlags = api.NewFlags(flagSet)
	)

	handler := func(args []string) error {
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		// Check for tag ID as positional argument
		if len(flagSet.Args()) != 1 {
			if len(flagSet.Args()) == 0 {
				return errors.New("provide exactly one tag ID as an argument")
			}
			return errors.New("provide exactly one tag ID as an argument (flags must come before positional arguments)")
		}
		tagID := flagSet.Arg(0)

		if *nameFlag == "" {
			return errors.New("provide a new name for the tag using -name flag (flags must come before positional arguments)")
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
			"id": tagID,
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

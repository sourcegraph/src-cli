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

  Get a prompt tag by name:

    	$ src prompts tags get go

`

	flagSet := flag.NewFlagSet("get", flag.ExitOnError)
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

		// Check for tag name as positional argument
		if len(args) == 0 {
			return errors.New("provide a tag name as an argument")
		}

		tagName := args[0]

		client := cfg.apiClient(apiFlags, flagSet.Output())

		query := `query PromptTags($query: String!) {
	promptTags(query: $query, first: 1) {
		nodes {
			...PromptTagFields
		}
	}
}` + promptTagFragment

		var result struct {
			PromptTags struct {
				Nodes []PromptTag
			}
		}

		vars := map[string]interface{}{
			"query": tagName,
		}

		if ok, err := client.NewRequest(query, vars).Do(context.Background(), &result); err != nil || !ok {
			return err
		}

		if len(result.PromptTags.Nodes) == 0 {
			return fmt.Errorf("no tag found with name '%s'", tagName)
		}

		// Display the tag information
		tag := result.PromptTags.Nodes[0]
		fmt.Printf("ID: %s\nName: %s\n", tag.ID, tag.Name)

		return nil
	}

	// Register the command.
	promptsTagsCommands = append(promptsTagsCommands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}

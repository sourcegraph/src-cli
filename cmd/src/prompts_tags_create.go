package main

import (
	"context"
	"flag"
	"fmt"
	"strings"

	"github.com/sourcegraph/sourcegraph/lib/errors"

	"github.com/sourcegraph/src-cli/internal/api"
)

func init() {
	usage := `
Examples:

  Create a new prompt tag:

    	$ src prompts tags create go

  Note: If a tag with this name already exists, the command will return the existing tag's ID.

`

	flagSet := flag.NewFlagSet("create", flag.ExitOnError)
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

		query := `mutation CreatePromptTag($input: PromptTagCreateInput!) {
	createPromptTag(input: $input) {
		...PromptTagFields
	}
}
` + promptTagFragment

		var result struct {
			CreatePromptTag PromptTag
		}
		if ok, err := client.NewRequest(query, map[string]interface{}{
			"input": map[string]interface{}{
				"name": tagName,
			},
		}).Do(context.Background(), &result); err != nil || !ok {
			// Check if this is a duplicate key error
			if err != nil && (strings.Contains(err.Error(), "duplicate key value") ||
				strings.Contains(err.Error(), "unique constraint")) {
				// Try to fetch the existing tag to provide more useful information
				existingTag, fetchErr := getExistingTag(client, tagName)
				if fetchErr == nil && existingTag != nil {
					return fmt.Errorf("a tag with the name '%s' already exists (ID: %s)", tagName, existingTag.ID)
				}
				return fmt.Errorf("a tag with the name '%s' already exists", tagName)
			}
			return err
		}

		fmt.Printf("Prompt tag created: %s\n", result.CreatePromptTag.ID)
		return nil
	}

	// Register the command.
	promptsTagsCommands = append(promptsTagsCommands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}

// getExistingTag tries to fetch an existing tag by name
func getExistingTag(client api.Client, name string) (*PromptTag, error) {
	query := `query PromptTags($query: String!) {
	promptTags(query: $query) {
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
		"query": name,
	}

	if ok, err := client.NewRequest(query, vars).Do(context.Background(), &result); err != nil || !ok {
		return nil, err
	}

	if len(result.PromptTags.Nodes) == 0 {
		return nil, nil
	}

	for _, tag := range result.PromptTags.Nodes {
		// Look for exact name match
		if tag.Name == name {
			return &tag, nil
		}
	}

	return nil, nil
}

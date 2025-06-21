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

  Get prompt details by ID:

    	$ src prompts get <prompt-id>

`

	flagSet := flag.NewFlagSet("get", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src prompts %s':\n", flagSet.Name())
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

		if len(flagSet.Args()) != 1 {
			return errors.New("provide exactly one prompt ID")
		}

		promptID := flagSet.Arg(0)

		client := cfg.apiClient(apiFlags, flagSet.Output())

		query := `query GetPrompt($id: ID!) {
	node(id: $id) {
		... on Prompt {
			...PromptFields
		}
	}
}
` + promptFragment

		vars := map[string]interface{}{
			"id": promptID,
		}

		var result struct {
			Node *Prompt `json:"node"`
		}

		if ok, err := client.NewRequest(query, vars).Do(context.Background(), &result); err != nil || !ok {
			return err
		}

		if result.Node == nil {
			return errors.Newf("prompt not found: %s", promptID)
		}

		p := result.Node
		tagNames := []string{}
		for _, tag := range p.Tags.Nodes {
			tagNames = append(tagNames, tag.Name)
		}

		fmt.Printf("ID: %s\n", p.ID)
		fmt.Printf("Name: %s\n", p.Name)
		fmt.Printf("Description: %s\n", p.Description)
		fmt.Printf("Content: %s\n", p.Definition.Text)
		fmt.Printf("Draft: %t\n", p.Draft)
		fmt.Printf("Visibility: %s\n", p.Visibility)
		fmt.Printf("Mode: %s\n", p.Mode)
		fmt.Printf("Auto-submit: %t\n", p.AutoSubmit)
		fmt.Printf("Recommended: %t\n", p.Recommended)

		if len(tagNames) > 0 {
			fmt.Printf("Tags: %s\n", joinStrings(tagNames, ", "))
		}

		return nil
	}

	// Register the command.
	promptsCommands = append(promptsCommands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}

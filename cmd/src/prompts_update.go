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

  Update a prompt's description:

    	$ src prompts update -id=<prompt-id> -name="Updated Name" -description="Updated description" [-content="Updated content"] [-tags=id1,id2] [-draft=false] [-auto-submit=false] [-mode=CHAT] [-recommended=false]

`

	flagSet := flag.NewFlagSet("update", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src prompts %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}
	var (
		idFlag          = flagSet.String("id", "", "The ID of the prompt to update")
		nameFlag        = flagSet.String("name", "", "The updated prompt name")
		descriptionFlag = flagSet.String("description", "", "Updated description of the prompt")
		contentFlag     = flagSet.String("content", "", "Updated prompt template text content")
		tagsFlag        = flagSet.String("tags", "", "Comma-separated list of tag IDs (replaces existing tags)")
		draftFlag       = flagSet.Bool("draft", false, "Whether the prompt is a draft")
		autoSubmitFlag  = flagSet.Bool("auto-submit", false, "Whether the prompt should be automatically executed in one click")
		modeFlag        = flagSet.String("mode", "CHAT", "Mode to execute prompt (CHAT, EDIT, or INSERT)")
		recommendedFlag = flagSet.Bool("recommended", false, "Whether the prompt is recommended")
		apiFlags        = api.NewFlags(flagSet)
	)

	handler := func(args []string) error {
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		if *idFlag == "" {
			return errors.New("provide the ID of the prompt to update")
		}

		if *nameFlag == "" {
			return errors.New("provide a name for the prompt")
		}

		if *descriptionFlag == "" {
			return errors.New("provide a description for the prompt")
		}

		if *contentFlag == "" {
			return errors.New("provide content for the prompt")
		}

		// Validate mode
		validModes := map[string]bool{"CHAT": true, "EDIT": true, "INSERT": true}
		mode := strings.ToUpper(*modeFlag)
		if !validModes[mode] {
			return errors.New("mode must be one of: CHAT, EDIT, or INSERT")
		}

		// Parse tags into array
		var tagIDs []string
		if *tagsFlag != "" {
			tagIDs = strings.Split(*tagsFlag, ",")
		}

		client := cfg.apiClient(apiFlags, flagSet.Output())

		query := `mutation UpdatePrompt(
	$id: ID!,
	$input: PromptUpdateInput!
) {
	updatePrompt(id: $id, input: $input) {
		...PromptFields
	}
}
` + promptFragment

		input := map[string]interface{}{
			"name":           *nameFlag,
			"description":    *descriptionFlag,
			"definitionText": *contentFlag,
			"draft":          *draftFlag,
			"autoSubmit":     *autoSubmitFlag,
			"mode":           mode,
			"recommended":    *recommendedFlag,
		}

		if len(tagIDs) > 0 {
			input["tags"] = tagIDs
		}

		var result struct {
			UpdatePrompt Prompt
		}
		if ok, err := client.NewRequest(query, map[string]interface{}{
			"id":    *idFlag,
			"input": input,
		}).Do(context.Background(), &result); err != nil || !ok {
			return err
		}

		fmt.Printf("Prompt updated: %s\n", result.UpdatePrompt.ID)
		return nil
	}

	// Register the command.
	promptsCommands = append(promptsCommands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}

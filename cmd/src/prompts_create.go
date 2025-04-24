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

  Create a prompt "Go Error Handling":

    	$ src prompts create -name="Go Error Handling" \
		-description="Best practices for Go error handling" \
		-content="When handling errors in Go..." \
		-owner=<owner-id>
`

	flagSet := flag.NewFlagSet("create", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src prompts %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}
	var (
		nameFlag        = flagSet.String("name", "", "The prompt name")
		descriptionFlag = flagSet.String("description", "", "Description of the prompt")
		contentFlag     = flagSet.String("content", "", "The prompt template text content")
		ownerFlag       = flagSet.String("owner", "", "The ID of the owner (user or organization)")
		tagsFlag        = flagSet.String("tags", "", "Comma-separated list of tag IDs")
		draftFlag       = flagSet.Bool("draft", false, "Whether the prompt is a draft")
		visibilityFlag  = flagSet.String("visibility", "PUBLIC", "Visibility of the prompt (PUBLIC or SECRET)")
		autoSubmitFlag  = flagSet.Bool("auto-submit", false, "Whether the prompt should be automatically executed in one click")
		modeFlag        = flagSet.String("mode", "CHAT", "Mode to execute prompt (CHAT, EDIT, or INSERT)")
		recommendedFlag = flagSet.Bool("recommended", false, "Whether the prompt is recommended")
		apiFlags        = api.NewFlags(flagSet)
	)

	handler := func(args []string) error {
		if err := flagSet.Parse(args); err != nil {
			return err
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
		if *ownerFlag == "" {
			return errors.New("provide an owner ID for the prompt")
		}

		// Validate mode
		validModes := map[string]bool{"CHAT": true, "EDIT": true, "INSERT": true}
		mode := strings.ToUpper(*modeFlag)
		if !validModes[mode] {
			return errors.New("mode must be one of: CHAT, EDIT, or INSERT")
		}

		// Validate visibility
		validVisibility := map[string]bool{"PUBLIC": true, "SECRET": true}
		visibility := strings.ToUpper(*visibilityFlag)
		if !validVisibility[visibility] {
			return errors.New("visibility must be either PUBLIC or SECRET")
		}

		// Parse tags into array
		var tagIDs []string
		if *tagsFlag != "" {
			tagIDs = strings.Split(*tagsFlag, ",")
		}

		client := cfg.apiClient(apiFlags, flagSet.Output())

		query := `mutation CreatePrompt(
	$input: PromptInput!
) {
	createPrompt(input: $input) {
		...PromptFields
	}
}
` + promptFragment

		input := map[string]interface{}{
			"name":           *nameFlag,
			"description":    *descriptionFlag,
			"definitionText": *contentFlag,
			"owner":          *ownerFlag,
			"draft":          *draftFlag,
			"visibility":     visibility,
			"autoSubmit":     *autoSubmitFlag,
			"mode":           mode,
			"recommended":    *recommendedFlag,
		}

		if len(tagIDs) > 0 {
			input["tags"] = tagIDs
		}

		var result struct {
			CreatePrompt Prompt
		}
		if ok, err := client.NewRequest(query, map[string]interface{}{
			"input": input,
		}).Do(context.Background(), &result); err != nil || !ok {
			return err
		}

		fmt.Printf("Prompt created: %s\n", result.CreatePrompt.ID)
		return nil
	}

	// Register the command.
	promptsCommands = append(promptsCommands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}

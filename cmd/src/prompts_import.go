package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/sourcegraph/sourcegraph/lib/errors"

	"github.com/sourcegraph/src-cli/internal/api"
)

const importTagName = "src_cli_import"

func init() {
	usage := `
Examples:

  Import prompts from a file (uses current user as owner):

    	$ src prompts import -i prompts.json

  Import prompts with a specific owner:

    	$ src prompts import -i prompts.json -owner=<owner-id>

  Perform a dry run without creating any prompts:

    	$ src prompts import -i prompts.json -dry-run

  Skip existing prompts with the same name:

    	$ src prompts import -i prompts.json -skip-existing

  Note: Prompts that already exist for the owner will be automatically skipped.
`

	flagSet := flag.NewFlagSet("import", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src prompts %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}
	var (
		inputFlag        = flagSet.String("i", "", "Input file path (required)")
		dryRunFlag       = flagSet.Bool("dry-run", false, "Validate without importing")
		skipExistingFlag = flagSet.Bool("skip-existing", false, "Skip prompts that already exist (based on name)")
		ownerFlag        = flagSet.String("owner", "", "The ID of the owner for all imported prompts (defaults to current user)")
		apiFlags         = api.NewFlags(flagSet)
	)

	handler := func(args []string) error {
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		if *inputFlag == "" {
			return errors.New("provide an input file path with -i")
		}

		client := cfg.apiClient(apiFlags, flagSet.Output())

		// If owner not specified, use the current user
		owner := *ownerFlag
		if owner == "" {
			// Get the current user ID
			currentUserID, err := getViewerUserID(context.Background(), client)
			if err != nil {
				return errors.New("unable to determine current user ID, please provide -owner explicitly")
			}
			owner = currentUserID
			fmt.Printf("Using current user as owner (ID: %s)\n", owner)
		}

		// Read the input file
		file, err := os.Open(*inputFlag)
		if err != nil {
			return fmt.Errorf("error opening input file: %w", err)
		}
		defer file.Close()

		// Parse the JSON
		data, err := io.ReadAll(file)
		if err != nil {
			return fmt.Errorf("error reading input file: %w", err)
		}

		var export PromptsExport
		if err := json.Unmarshal(data, &export); err != nil {
			return fmt.Errorf("error parsing JSON: %w", err)
		}

		// Validate the export data
		if export.Version == "" {
			return errors.New("invalid export file: missing version")
		}

		if len(export.Prompts) == 0 {
			return errors.New("no prompts found in the export file")
		}

		// Get or create the import tag
		importTagID, err := getOrCreateTag(client, importTagName)
		if err != nil {
			return fmt.Errorf("error getting/creating import tag: %w", err)
		}

		// Fetch all existing tags to build a mapping
		tagNameToID, err := getTagMapping(client)
		if err != nil {
			return fmt.Errorf("error fetching existing tags: %w", err)
		}

		// In case of skip-existing, get all existing prompt names
		existingPromptNames := make(map[string]bool)
		if *skipExistingFlag {
			names, err := getAllPromptNames(client)
			if err != nil {
				return fmt.Errorf("error fetching existing prompts: %w", err)
			}
			existingPromptNames = names
		}

		// Dry run message
		if *dryRunFlag {
			fmt.Printf("Dry run: would import %d prompts\n", len(export.Prompts))
		}

		// Process each prompt
		var importedCount, skippedCount int
		for _, prompt := range export.Prompts {
			// Skip if prompt with same name exists and skip-existing is enabled
			if *skipExistingFlag && existingPromptNames[prompt.Name] {
				fmt.Printf("Skipping prompt '%s' as it already exists\n", prompt.Name)
				skippedCount++
				continue
			}

			// Process tags for this prompt
			tagIDs := []string{importTagID} // Always add the import tag
			for _, tag := range prompt.Tags.Nodes {
				tagID, created, err := resolveTagID(client, tag.Name, tagNameToID)
				if err != nil {
					return fmt.Errorf("error resolving tag '%s': %w", tag.Name, err)
				}

				if created {
					// Update our mapping with the new tag
					tagNameToID[tag.Name] = tagID
					if !*dryRunFlag {
						fmt.Printf("Created new tag: %s\n", tag.Name)
					}
				}

				tagIDs = append(tagIDs, tagID)
			}

			// Skip actual creation in dry run mode
			if *dryRunFlag {
				fmt.Printf("Would import prompt: %s\n", prompt.Name)
				importedCount++
				continue
			}

			// Create the prompt
			created, err := createPrompt(client, prompt, owner, tagIDs)
			if err != nil {
				// Check if this is a duplicate prompt error
				if strings.Contains(err.Error(), "already exists") {
					fmt.Printf("Skipping prompt '%s' as it already exists\n", prompt.Name)
					skippedCount++
					continue
				}
				return fmt.Errorf("error creating prompt '%s': %w", prompt.Name, err)
			}

			fmt.Printf("Imported prompt: %s (ID: %s)\n", prompt.Name, created.ID)
			importedCount++
		}

		// Print summary
		action := "Imported"
		if *dryRunFlag {
			action = "Would import"
		}
		fmt.Printf("%s %d prompts", action, importedCount)
		if skippedCount > 0 {
			fmt.Printf(" (skipped %d existing)", skippedCount)
		}
		fmt.Println()

		return nil
	}

	// Register the command.
	promptsCommands = append(promptsCommands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}

// getOrCreateTag gets a tag by name or creates it if it doesn't exist
func getOrCreateTag(client api.Client, name string) (string, error) {
	// First try to get the tag by name
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
		"query": name,
	}

	if ok, err := client.NewRequest(query, vars).Do(context.Background(), &result); err != nil || !ok {
		return "", err
	}

	// If the tag exists, return its ID
	if len(result.PromptTags.Nodes) > 0 {
		return result.PromptTags.Nodes[0].ID, nil
	}

	// Tag doesn't exist, create it
	mutation := `mutation CreatePromptTag($input: PromptTagCreateInput!) {
	createPromptTag(input: $input) {
		...PromptTagFields
	}
}` + promptTagFragment

	var createResult struct {
		CreatePromptTag PromptTag
	}

	if ok, err := client.NewRequest(mutation, map[string]interface{}{
		"input": map[string]interface{}{
			"name": name,
		},
	}).Do(context.Background(), &createResult); err != nil || !ok {
		return "", err
	}

	return createResult.CreatePromptTag.ID, nil
}

// getTagMapping fetches all tags and returns a map of tag names to IDs
func getTagMapping(client api.Client) (map[string]string, error) {
	query := `query PromptTags {
	promptTags(first: 1000) {
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

	if ok, err := client.NewRequest(query, nil).Do(context.Background(), &result); err != nil || !ok {
		return nil, err
	}

	tagMap := make(map[string]string)
	for _, tag := range result.PromptTags.Nodes {
		tagMap[tag.Name] = tag.ID
	}

	return tagMap, nil
}

// getAllPromptNames fetches all prompt names and returns them as a map for quick lookup
func getAllPromptNames(client api.Client) (map[string]bool, error) {
	query := `query AllPromptNames {
	prompts(first: 1000) {
		nodes {
			name
		}
	}
}`

	var result struct {
		Prompts struct {
			Nodes []struct {
				Name string
			}
		}
	}

	if ok, err := client.NewRequest(query, nil).Do(context.Background(), &result); err != nil || !ok {
		return nil, err
	}

	nameMap := make(map[string]bool)
	for _, prompt := range result.Prompts.Nodes {
		nameMap[prompt.Name] = true
	}

	return nameMap, nil
}

// resolveTagID resolves a tag name to an ID, creating the tag if it doesn't exist
// Returns the tag ID, a boolean indicating whether a new tag was created, and an error
func resolveTagID(client api.Client, name string, tagMap map[string]string) (string, bool, error) {
	// Check if we already have this tag
	if id, ok := tagMap[name]; ok {
		return id, false, nil
	}

	// Create the tag
	mutation := `mutation CreatePromptTag($input: PromptTagCreateInput!) {
	createPromptTag(input: $input) {
		...PromptTagFields
	}
}` + promptTagFragment

	var result struct {
		CreatePromptTag PromptTag
	}

	if ok, err := client.NewRequest(mutation, map[string]interface{}{
		"input": map[string]interface{}{
			"name": name,
		},
	}).Do(context.Background(), &result); err != nil || !ok {
		return "", false, err
	}

	return result.CreatePromptTag.ID, true, nil
}

// createPrompt creates a new prompt with the given properties
func createPrompt(client api.Client, prompt Prompt, ownerID string, tagIDs []string) (*Prompt, error) {
	mutation := `mutation CreatePrompt(
	$input: PromptInput!
) {
	createPrompt(input: $input) {
		...PromptFields
	}
}` + promptFragment

	// Build input from the prompt
	input := map[string]interface{}{
		"name":           prompt.Name,
		"description":    prompt.Description,
		"definitionText": prompt.Definition.Text,
		"owner":          ownerID,
		"draft":          prompt.Draft,
		"visibility":     prompt.Visibility,
		"autoSubmit":     prompt.AutoSubmit,
		"mode":           prompt.Mode,
		"recommended":    prompt.Recommended,
		"tags":           tagIDs,
	}

	var result struct {
		CreatePrompt Prompt
	}

	if ok, err := client.NewRequest(mutation, map[string]interface{}{
		"input": input,
	}).Do(context.Background(), &result); err != nil || !ok {
		// Check if this is a duplicate prompt error
		if err != nil && (strings.Contains(err.Error(), "duplicate key value") ||
			strings.Contains(err.Error(), "prompts_name_is_unique_in_owner_user")) {
			return nil, fmt.Errorf("a prompt with the name '%s' already exists for this owner", prompt.Name)
		}
		return nil, err
	}

	return &result.CreatePrompt, nil
}

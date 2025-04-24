package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/sourcegraph/src-cli/internal/api"
)

type PromptsExport struct {
	Version    string   `json:"version"`
	Prompts    []Prompt `json:"prompts"`
	ExportDate string   `json:"exportDate"`
}

func init() {
	usage := `
Examples:

  Export all prompts to a file:

    	$ src prompts export -o prompts.json

  Export prompts with specific tags:

    	$ src prompts export -o prompts.json -tags=go,python

  Export with pretty JSON formatting:

    	$ src prompts export -o prompts.json -format=pretty

  Export to stdout:

    	$ src prompts export
`

	flagSet := flag.NewFlagSet("export", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src prompts %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}
	var (
		outputFlag = flagSet.String("o", "", "Output file path (defaults to stdout if not specified)")
		tagsFlag   = flagSet.String("tags", "", "Comma-separated list of tag names to filter by")
		formatFlag = flagSet.String("format", "compact", "JSON format: 'pretty' or 'compact'")
		apiFlags   = api.NewFlags(flagSet)
	)

	handler := func(args []string) error {
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		// Validate format flag
		format := *formatFlag
		if format != "pretty" && format != "compact" {
			return fmt.Errorf("format must be either 'pretty' or 'compact'")
		}

		client := cfg.apiClient(apiFlags, flagSet.Output())

		// Parse tags into array
		var tagNames []string
		if *tagsFlag != "" {
			tagNames = parseIDsFromString(*tagsFlag)
		}

		// If tags are specified, first resolve tag names to IDs
		var tagIDs []string
		if len(tagNames) > 0 {
			ids, err := resolveTagNamesToIDs(client, tagNames)
			if err != nil {
				return err
			}
			tagIDs = ids
		}

		// Fetch all prompts using pagination
		allPrompts, err := fetchAllPrompts(client, tagIDs)
		if err != nil {
			return err
		}

		// Create export data structure
		export := PromptsExport{
			Version:    "1.0",
			Prompts:    allPrompts,
			ExportDate: time.Now().UTC().Format(time.RFC3339),
		}

		// Marshal to JSON
		var jsonData []byte
		var jsonErr error
		if format == "pretty" {
			jsonData, jsonErr = json.MarshalIndent(export, "", "  ")
		} else {
			jsonData, jsonErr = json.Marshal(export)
		}

		if jsonErr != nil {
			return fmt.Errorf("error marshaling JSON: %w", jsonErr)
		}

		// Determine output destination
		var out io.Writer
		if *outputFlag == "" {
			out = flagSet.Output()
		} else {
			file, err := os.Create(*outputFlag)
			if err != nil {
				return fmt.Errorf("error creating output file: %w", err)
			}
			defer file.Close()
			out = file
		}

		// Write output
		_, err = out.Write(jsonData)
		if err != nil {
			return fmt.Errorf("error writing output: %w", err)
		}

		// Print summary if output is to a file
		if *outputFlag != "" {
			fmt.Printf("Exported %d prompts to %s\n", len(allPrompts), *outputFlag)
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

// fetchAllPrompts fetches all prompts from the API using pagination
func fetchAllPrompts(client api.Client, tagIDs []string) ([]Prompt, error) {
	var allPrompts []Prompt
	after := ""
	hasNextPage := true

	for hasNextPage {
		// Build the query dynamically based on which parameters we have
		queryStr := "query Prompts($first: Int!, $includeDrafts: Boolean"
		promptsParams := "first: $first, includeDrafts: $includeDrafts"

		// Add optional parameters
		if after != "" {
			queryStr += ", $after: String"
			promptsParams += ", after: $after"
		}
		if len(tagIDs) > 0 {
			queryStr += ", $tags: [ID!]"
			promptsParams += ", tags: $tags"
		}

		// Close the query definition
		queryStr += ") {"

		query := queryStr + `
	prompts(
		` + promptsParams + `
	) {
		totalCount
		nodes {
			...PromptFields
		}
		pageInfo {
			hasNextPage
			endCursor
		}
	}
}` + promptFragment

		// Initialize variables with the required parameters
		vars := map[string]interface{}{
			"first":         100, // Get max prompts per page
			"includeDrafts": true,
		}

		// Add optional parameters
		if after != "" {
			vars["after"] = after
		}
		if len(tagIDs) > 0 {
			vars["tags"] = tagIDs
		}

		var result struct {
			Prompts struct {
				TotalCount int `json:"totalCount"`
				Nodes      []Prompt
				PageInfo   struct {
					HasNextPage bool   `json:"hasNextPage"`
					EndCursor   string `json:"endCursor"`
				} `json:"pageInfo"`
			}
		}

		if ok, err := client.NewRequest(query, vars).Do(context.Background(), &result); err != nil || !ok {
			return nil, err
		}

		// Add current page prompts to the result
		allPrompts = append(allPrompts, result.Prompts.Nodes...)

		// Update pagination info
		hasNextPage = result.Prompts.PageInfo.HasNextPage
		if hasNextPage {
			after = result.Prompts.PageInfo.EndCursor
		}
	}

	return allPrompts, nil
}

// resolveTagNamesToIDs resolves tag names to their IDs
func resolveTagNamesToIDs(client api.Client, tagNames []string) ([]string, error) {
	// Query to get all tags
	query := `query PromptTags {
	promptTags {
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

	// Create a map of tag names to IDs
	tagNameToID := make(map[string]string)
	for _, tag := range result.PromptTags.Nodes {
		tagNameToID[tag.Name] = tag.ID
	}

	// Resolve IDs for requested tag names
	var tagIDs []string
	var missingTags []string
	for _, name := range tagNames {
		if id, ok := tagNameToID[name]; ok {
			tagIDs = append(tagIDs, id)
		} else {
			missingTags = append(missingTags, name)
		}
	}

	// If we have missing tags, return an error
	if len(missingTags) > 0 {
		return nil, fmt.Errorf("the following tags were not found: %v", missingTags)
	}

	return tagIDs, nil
}

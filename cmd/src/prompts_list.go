package main

import (
	"context"
	"flag"
	"fmt"
	"strings"

	"github.com/sourcegraph/src-cli/internal/api"
)

func init() {
	usage := `
Examples:

  List all prompts:

    	$ src prompts list

  Search prompts by name or contents:

    	$ src prompts list -query="error handling"

  Filter prompts by tag:

    	$ src prompts list -tags=id1,id2

  List prompts for a specific owner:

    	$ src prompts list -owner=<owner-id>

  Paginate through results:

    	$ src prompts list -after=<cursor>

`

	flagSet := flag.NewFlagSet("list", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src prompts %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}
	var (
		queryFlag           = flagSet.String("query", "", "Search prompts by name, description, or content")
		ownerFlag           = flagSet.String("owner", "", "Filter by prompt owner (a namespace, either a user or organization)")
		tagsFlag            = flagSet.String("tags", "", "Comma-separated list of tag IDs to filter by")
		affilatedFlag       = flagSet.Bool("affiliated", false, "Filter to only prompts owned by the viewer or viewer's organizations")
		includeDraftsFlag   = flagSet.Bool("include-drafts", true, "Whether to include draft prompts")
		recommendedOnlyFlag = flagSet.Bool("recommended-only", false, "Whether to include only recommended prompts")
		builtinOnlyFlag     = flagSet.Bool("builtin-only", false, "Whether to include only builtin prompts")
		includeBuiltinFlag  = flagSet.Bool("include-builtin", false, "Whether to include builtin prompts")
		limitFlag           = flagSet.Int("limit", 100, "Maximum number of prompts to list")
		afterFlag           = flagSet.String("after", "", "Cursor for pagination (from previous page's endCursor)")
		apiFlags            = api.NewFlags(flagSet)
	)

	handler := func(args []string) error {
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		client := cfg.apiClient(apiFlags, flagSet.Output())

		// Parse tags into array
		var tagIDs []string
		if *tagsFlag != "" {
			tagIDs = append(tagIDs, parseIDsFromString(*tagsFlag)...)
		}

		// Build the query dynamically based on which parameters we have
		queryStr := "query Prompts($first: Int!, $includeDrafts: Boolean"
		promptsParams := "first: $first, includeDrafts: $includeDrafts"

		// Add optional parameters to the query
		if *queryFlag != "" {
			queryStr += ", $query: String"
			promptsParams += ", query: $query"
		}
		if *ownerFlag != "" {
			queryStr += ", $owner: ID"
			promptsParams += ", owner: $owner"
		}
		if *affilatedFlag {
			queryStr += ", $viewerIsAffiliated: Boolean"
			promptsParams += ", viewerIsAffiliated: $viewerIsAffiliated"
		}
		if *recommendedOnlyFlag {
			queryStr += ", $recommendedOnly: Boolean"
			promptsParams += ", recommendedOnly: $recommendedOnly"
		}
		if *builtinOnlyFlag {
			queryStr += ", $builtinOnly: Boolean"
			promptsParams += ", builtinOnly: $builtinOnly"
		}
		if *includeBuiltinFlag {
			queryStr += ", $includeBuiltin: Boolean"
			promptsParams += ", includeBuiltin: $includeBuiltin"
		}
		if *afterFlag != "" {
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
			"first":         *limitFlag,
			"includeDrafts": *includeDraftsFlag,
		}

		// Only add optional parameters if they're provided
		if *queryFlag != "" {
			vars["query"] = *queryFlag
		}
		if *ownerFlag != "" {
			vars["owner"] = *ownerFlag
		}
		if *affilatedFlag {
			vars["viewerIsAffiliated"] = true
		}
		if *recommendedOnlyFlag {
			vars["recommendedOnly"] = true
		}
		if *builtinOnlyFlag {
			vars["builtinOnly"] = true
		}
		if *includeBuiltinFlag {
			vars["includeBuiltin"] = true
		}
		if *afterFlag != "" {
			vars["after"] = *afterFlag
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
			return err
		}

		fmt.Printf("Showing %d of %d prompts\n\n", len(result.Prompts.Nodes), result.Prompts.TotalCount)

		for _, p := range result.Prompts.Nodes {
			tagNames := []string{}
			for _, tag := range p.Tags.Nodes {
				tagNames = append(tagNames, tag.Name)
			}

			fmt.Printf("ID: %s\nName: %s\nDescription: %s\n", p.ID, p.Name, p.Description)
			fmt.Printf("Draft: %t | Visibility: %s | Mode: %s\n", p.Draft, p.Visibility, p.Mode)
			if len(tagNames) > 0 {
				fmt.Printf("Tags: %s\n", joinStrings(tagNames, ", "))
			}
			fmt.Println()
		}

		if result.Prompts.PageInfo.HasNextPage {
			fmt.Printf("More results available. Use -after=%s to fetch the next page.\n", result.Prompts.PageInfo.EndCursor)
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

// Helper function to parse comma-separated IDs
func parseIDsFromString(s string) []string {
	if s == "" {
		return nil
	}

	split := strings.Split(s, ",")
	result := make([]string, 0, len(split))

	for _, id := range split {
		trimmed := strings.TrimSpace(id)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	return result
}

// Helper function to join string slices
func joinStrings(s []string, sep string) string {
	return strings.Join(s, sep)
}

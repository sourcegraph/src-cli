package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/sourcegraph/src-cli/internal/api"
)

func init() {
	usage := `
Examples:

  List all prompt tags:

    	$ src prompts tags list

  Search for prompt tags by name:

    	$ src prompts tags list -query="go"

  Paginate through results:

    	$ src prompts tags list -after=<cursor>

`

	flagSet := flag.NewFlagSet("list", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src prompts tags %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}
	var (
		queryFlag = flagSet.String("query", "", "Search prompt tags by name")
		limitFlag = flagSet.Int("limit", 100, "Maximum number of tags to list")
		afterFlag = flagSet.String("after", "", "Cursor for pagination (from previous page's endCursor)")
		apiFlags  = api.NewFlags(flagSet)
	)

	handler := func(args []string) error {
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		client := cfg.apiClient(apiFlags, flagSet.Output())

		// Build query dynamically based on provided parameters
		queryStr := "query PromptTags($first: Int!"
		tagsParams := "first: $first"

		if *queryFlag != "" {
			queryStr += ", $query: String"
			tagsParams += ", query: $query"
		}

		if *afterFlag != "" {
			queryStr += ", $after: String"
			tagsParams += ", after: $after"
		}

		// Close the query definition
		queryStr += ") {"

		query := queryStr + `
	promptTags(
		` + tagsParams + `
	) {
		totalCount
		nodes {
			...PromptTagFields
		}
		pageInfo {
			hasNextPage
			endCursor
		}
	}
}
` + promptTagFragment

		// Initialize with required parameters
		vars := map[string]interface{}{
			"first": *limitFlag,
		}

		// Only add optional parameters when provided
		if *queryFlag != "" {
			vars["query"] = *queryFlag
		}
		if *afterFlag != "" {
			vars["after"] = *afterFlag
		}

		var result struct {
			PromptTags struct {
				TotalCount int `json:"totalCount"`
				Nodes      []PromptTag
				PageInfo   struct {
					HasNextPage bool   `json:"hasNextPage"`
					EndCursor   string `json:"endCursor"`
				} `json:"pageInfo"`
			}
		}

		if ok, err := client.NewRequest(query, vars).Do(context.Background(), &result); err != nil || !ok {
			return err
		}

		fmt.Printf("Showing %d of %d prompt tags\n\n", len(result.PromptTags.Nodes), result.PromptTags.TotalCount)

		for _, tag := range result.PromptTags.Nodes {
			fmt.Printf("ID: %s\nName: %s\n\n", tag.ID, tag.Name)
		}

		if result.PromptTags.PageInfo.HasNextPage {
			fmt.Printf("More results available. Use -after=%s to fetch the next page.\n", result.PromptTags.PageInfo.EndCursor)
		}

		return nil
	}

	// Register the command.
	promptsTagsCommands = append(promptsTagsCommands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}

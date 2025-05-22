package main

import (
	"context"
	"flag"
	"fmt"
	"strings"

	"github.com/sourcegraph/src-cli/internal/api"
)

// availablePromptColumns defines the available column names for output
var availablePromptColumns = map[string]bool{
	"id":          true,
	"name":        true,
	"description": true,
	"draft":       true,
	"visibility":  true,
	"mode":        true,
	"tags":        true,
}

// defaultPromptColumns defines the default columns to display
var defaultPromptColumns = []string{"id", "name", "visibility", "tags"}

// displayPrompts formats and outputs multiple prompts
func displayPrompts(prompts []Prompt, columns []string, asJSON bool) error {
	if asJSON {
		return outputAsJSON(prompts)
	}

	// Collect all data first to calculate column widths
	allRows := make([][]string, 0, len(prompts)+1)

	// Add header row
	headers := make([]string, 0, len(columns))
	for _, col := range columns {
		headers = append(headers, strings.ToUpper(col))
	}
	allRows = append(allRows, headers)

	// Collect all data rows
	for _, p := range prompts {
		row := make([]string, 0, len(columns))

		// Prepare tag names for display
		tagNames := []string{}
		for _, tag := range p.Tags.Nodes {
			tagNames = append(tagNames, tag.Name)
		}
		tagsStr := joinStrings(tagNames, ", ")

		for _, col := range columns {
			switch col {
			case "id":
				row = append(row, p.ID)
			case "name":
				row = append(row, p.Name)
			case "description":
				row = append(row, p.Description)
			case "draft":
				row = append(row, fmt.Sprintf("%t", p.Draft))
			case "visibility":
				row = append(row, p.Visibility)
			case "mode":
				row = append(row, p.Mode)
			case "tags":
				row = append(row, tagsStr)
			}
		}
		allRows = append(allRows, row)
	}

	// Calculate max width for each column
	colWidths := make([]int, len(columns))
	for _, row := range allRows {
		for i, cell := range row {
			if len(cell) > colWidths[i] {
				colWidths[i] = len(cell)
			}
		}
	}

	// Print all rows with proper padding
	for i, row := range allRows {
		for j, cell := range row {
			fmt.Print(cell)
			// Add padding (at least 2 spaces between columns)
			padding := colWidths[j] - len(cell) + 2
			fmt.Print(strings.Repeat(" ", padding))
		}
		fmt.Println()

		// Add separator line after headers
		if i == 0 {
			for j, width := range colWidths {
				fmt.Print(strings.Repeat("-", width))
				if j < len(colWidths)-1 {
					fmt.Print("  ")
				}
			}
			fmt.Println()
		}
	}

	return nil
}

// parsePromptColumns parses and validates the columns flag
func parsePromptColumns(columnsFlag string) []string {
	if columnsFlag == "" {
		return defaultPromptColumns
	}

	columns := strings.Split(columnsFlag, ",")
	var validColumns []string

	for _, col := range columns {
		col = strings.ToLower(strings.TrimSpace(col))
		if availablePromptColumns[col] {
			validColumns = append(validColumns, col)
		}
	}

	if len(validColumns) == 0 {
		return defaultPromptColumns
	}

	return validColumns
}

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
    	
  Select specific columns to display:
  
    	$ src prompts list -c id,name,visibility,tags
    	
  Output results as JSON:
  
    	$ src prompts list -json

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
		columnsFlag         = flagSet.String("c", strings.Join(defaultPromptColumns, ","), "Comma-separated list of columns to display. Available: id,name,description,draft,visibility,mode,tags")
		jsonFlag            = flagSet.Bool("json", false, "Output results as JSON for programmatic access")
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

		// Parse columns
		columns := parsePromptColumns(*columnsFlag)

		fmt.Printf("Showing %d of %d prompts\n\n", len(result.Prompts.Nodes), result.Prompts.TotalCount)

		// Display prompts in tabular format
		if err := displayPrompts(result.Prompts.Nodes, columns, *jsonFlag); err != nil {
			return err
		}

		if result.Prompts.PageInfo.HasNextPage {
			fmt.Printf("\nMore results available. Use -after=%s to fetch the next page.\n", result.Prompts.PageInfo.EndCursor)
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

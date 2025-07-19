package main

import (
	"context"
	"flag"
	"fmt"
	"strings"

	"github.com/sourcegraph/src-cli/internal/api"
)

// availableTagColumns defines the available column names for output
var availableTagColumns = map[string]bool{
	"id":   true,
	"name": true,
}

// defaultTagColumns defines the default columns to display
var defaultTagColumns = []string{"id", "name"}

// displayPromptTags formats and outputs multiple prompt tags
func displayPromptTags(tags []PromptTag, columns []string, asJSON bool) error {
	if asJSON {
		return outputAsJSON(tags)
	}

	// Collect all data first to calculate column widths
	allRows := make([][]string, 0, len(tags)+1)

	// Add header row
	headers := make([]string, 0, len(columns))
	for _, col := range columns {
		headers = append(headers, strings.ToUpper(col))
	}
	allRows = append(allRows, headers)

	// Collect all data rows
	for _, tag := range tags {
		row := make([]string, 0, len(columns))

		for _, col := range columns {
			switch col {
			case "id":
				row = append(row, tag.ID)
			case "name":
				row = append(row, tag.Name)
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

// parseTagColumns parses and validates the columns flag
func parseTagColumns(columnsFlag string) []string {
	if columnsFlag == "" {
		return defaultTagColumns
	}

	columns := strings.Split(columnsFlag, ",")
	var validColumns []string

	for _, col := range columns {
		col = strings.ToLower(strings.TrimSpace(col))
		if availableTagColumns[col] {
			validColumns = append(validColumns, col)
		}
	}

	if len(validColumns) == 0 {
		return defaultTagColumns
	}

	return validColumns
}

func init() {
	usage := `
Examples:

  List all prompt tags:

    	$ src prompts tags list

  Search for prompt tags by name:

    	$ src prompts tags list -query="go"

  Paginate through results:

    	$ src prompts tags list -after=<cursor>
    	
  Select specific columns to display:
  
    	$ src prompts tags list -c id,name
    	
  Output results as JSON:
  
    	$ src prompts tags list -json

`

	flagSet := flag.NewFlagSet("list", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src prompts tags %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}
	var (
		queryFlag   = flagSet.String("query", "", "Search prompt tags by name")
		limitFlag   = flagSet.Int("limit", 100, "Maximum number of tags to list")
		afterFlag   = flagSet.String("after", "", "Cursor for pagination (from previous page's endCursor)")
		columnsFlag = flagSet.String("c", strings.Join(defaultTagColumns, ","), "Comma-separated list of columns to display. Available: id,name")
		jsonFlag    = flagSet.Bool("json", false, "Output results as JSON for programmatic access")
		apiFlags    = api.NewFlags(flagSet)
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

		// Parse columns
		columns := parseTagColumns(*columnsFlag)

		fmt.Printf("Showing %d of %d prompt tags\n\n", len(result.PromptTags.Nodes), result.PromptTags.TotalCount)

		// Display tags in tabular format
		if err := displayPromptTags(result.PromptTags.Nodes, columns, *jsonFlag); err != nil {
			return err
		}

		if result.PromptTags.PageInfo.HasNextPage {
			fmt.Printf("\nMore results available. Use -after=%s to fetch the next page.\n", result.PromptTags.PageInfo.EndCursor)
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

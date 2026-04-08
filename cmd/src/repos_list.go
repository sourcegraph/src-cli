package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"strings"

	"github.com/sourcegraph/src-cli/internal/api"
)

type reposListOptions struct {
	first      int
	query      string
	cloned     bool
	notCloned  bool
	indexed    bool
	notIndexed bool
	orderBy    string
	descending bool
}

type repositoriesListResult struct {
	Data struct {
		Repositories struct {
			Nodes []Repository `json:"nodes"`
		} `json:"repositories"`
	} `json:"data"`
	Errors []json.RawMessage `json:"errors,omitempty"`
}

// listRepositories returns the repositories from the response, any GraphQL
// errors returned alongside data (should be treated as warnings), and
// a hard error when the query fails without usable repository data.
func listRepositories(ctx context.Context, client api.Client, params reposListOptions) ([]Repository, api.GraphQlErrors, error) {
	query := `query Repositories(
  $first: Int,
  $query: String,
  $cloned: Boolean,
  $notCloned: Boolean,
  $indexed: Boolean,
  $notIndexed: Boolean,
  $orderBy: RepositoryOrderBy,
  $descending: Boolean,
) {
  repositories(
    first: $first,
    query: $query,
    cloned: $cloned,
    notCloned: $notCloned,
    indexed: $indexed,
    notIndexed: $notIndexed,
    orderBy: $orderBy,
    descending: $descending,
  ) {
    nodes {
      ...RepositoryFields
    }
  }
}
` + repositoryFragment

	var result repositoriesListResult
	ok, err := client.NewRequest(query, map[string]any{
		"first":      api.NullInt(params.first),
		"query":      api.NullString(params.query),
		"cloned":     params.cloned,
		"notCloned":  params.notCloned,
		"indexed":    params.indexed,
		"notIndexed": params.notIndexed,
		"orderBy":    params.orderBy,
		"descending": params.descending,
	}).DoRaw(ctx, &result)
	if err != nil || !ok {
		return nil, nil, err
	}
	repos := result.Data.Repositories.Nodes
	if len(result.Errors) == 0 {
		return repos, nil, nil
	}

	errors := api.NewGraphQlErrors(result.Errors)
	if len(repos) > 0 {
		return repos, errors, nil
	}

	return nil, nil, errors
}

func gqlErrorPathString(pathSegment any) (string, bool) {
	value, ok := pathSegment.(string)
	return value, ok
}

func gqlErrorIndex(pathSegment any) (int, bool) {
	switch value := pathSegment.(type) {
	case float64:
		index := int(value)
		return index, float64(index) == value && index >= 0
	case int:
		return value, value >= 0
	default:
		return 0, false
	}
}

func gqlWarningPath(graphQLError *api.GraphQlError) string {
	path, err := graphQLError.Path()
	if err != nil || len(path) == 0 {
		return ""
	}

	var b strings.Builder
	for _, pathSegment := range path {
		if segment, ok := gqlErrorPathString(pathSegment); ok {
			if b.Len() > 0 {
				b.WriteByte('.')
			}
			b.WriteString(segment)
			continue
		}

		if index, ok := gqlErrorIndex(pathSegment); ok {
			fmt.Fprintf(&b, "[%d]", index)
		}
	}

	return b.String()
}

func gqlWarningMessage(graphQLError *api.GraphQlError) string {
	message, err := graphQLError.Message()
	if err != nil || message == "" {
		return graphQLError.Error()
	}
	return message
}

func formatRepositoryListWarnings(warnings api.GraphQlErrors) string {
	var b strings.Builder
	fmt.Fprintf(&b, "warnings: %d errors during listing\n", len(warnings))
	for _, warning := range warnings {
		path := gqlWarningPath(warning)
		message := gqlWarningMessage(warning)
		if path != "" {
			fmt.Fprintf(&b, "%s - %s\n", path, message)
		} else {
			fmt.Fprintf(&b, "%s\n", message)
		}
		fmt.Fprintf(&b, "%s\n", warning.Error())
	}
	return b.String()
}

func init() {
	usage := `
Examples:

  List repositories:

    	$ src repos list

  Print JSON description of repositories list:

    	$ src repos list -f '{{.|json}}'

  List *all* repositories (may be slow!):

    	$ src repos list -first='-1'

  List repositories whose names match the query:

    	$ src repos list -query='myquery'

`

	flagSet := flag.NewFlagSet("list", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src repos %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}
	var (
		firstFlag = flagSet.Int("first", 1000, "Returns the first n repositories from the list. (use -1 for unlimited)")
		queryFlag = flagSet.String("query", "", `Returns repositories whose names match the query. (e.g. "myorg/")`)
		// TODO: add support for "names" field.
		clonedFlag           = flagSet.Bool("cloned", true, "Include cloned repositories.")
		notClonedFlag        = flagSet.Bool("not-cloned", true, "Include repositories that are not yet cloned and for which cloning is not in progress.")
		indexedFlag          = flagSet.Bool("indexed", true, "Include repositories that have a text search index.")
		notIndexedFlag       = flagSet.Bool("not-indexed", true, "Include repositories that do not have a text search index.")
		orderByFlag          = flagSet.String("order-by", "name", `How to order the results; possible choices are: "name", "created-at"`)
		descendingFlag       = flagSet.Bool("descending", false, "Whether or not results should be in descending order.")
		namesWithoutHostFlag = flagSet.Bool("names-without-host", false, "Whether or not repository names should be printed without the hostname (or other first path component). If set, -f is ignored.")
		formatFlag           = flagSet.String("f", "{{.Name}}", `Format for the output, using the syntax of Go package text/template. (e.g. "{{.ID}}: {{.Name}}") or "{{.|json}}")`)
		apiFlags             = api.NewFlags(flagSet)
	)

	handler := func(args []string) error {
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		client := cfg.apiClient(apiFlags, flagSet.Output())

		tmpl, err := parseTemplate(*formatFlag)
		if err != nil {
			return err
		}

		var orderBy string
		switch *orderByFlag {
		case "name":
			orderBy = "REPOSITORY_NAME"
		case "created-at":
			orderBy = "REPO_CREATED_AT"
		default:
			return fmt.Errorf("invalid -order-by flag value: %q", *orderByFlag)
		}

		// if we get repos and errors during a listing, we consider the errors as warnings and the data partially complete
		repos, warnings, err := listRepositories(context.Background(), client, reposListOptions{
			first:      *firstFlag,
			query:      *queryFlag,
			cloned:     *clonedFlag,
			notCloned:  *notClonedFlag,
			indexed:    *indexedFlag,
			notIndexed: *notIndexedFlag,
			orderBy:    orderBy,
			descending: *descendingFlag,
		})
		if err != nil {
			return err
		}

		for _, repo := range repos {
			if *namesWithoutHostFlag {
				firstSlash := strings.Index(repo.Name, "/")
				fmt.Println(repo.Name[firstSlash+len("/"):])
				continue
			}

			if err := execTemplate(tmpl, repo); err != nil {
				return err
			}
		}
		if len(warnings) > 0 {
			if *verbose {
				fmt.Fprint(flagSet.Output(), formatRepositoryListWarnings(warnings))
			} else {
				fmt.Fprintf(flagSet.Output(), "warning: %d errors during listing; rerun with -v to inspect them\n", len(warnings))
			}
		}
		return nil
	}

	// Register the command.
	reposCommands = append(reposCommands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}

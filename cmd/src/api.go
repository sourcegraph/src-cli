package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/sourcegraph/src-cli/internal/clicompat"
	"github.com/sourcegraph/src-cli/internal/cmderrors"

	"github.com/mattn/go-isatty"
	"github.com/urfave/cli/v3"
)

const apiExamples = `
Exit codes:

  0: Success
  1: General failures (connection issues, invalid HTTP response, etc.)
  2: GraphQL error response

Examples:

  Run queries (identical behavior):

    	$ echo 'query { currentUser { username } }' | src api
    	$ src api -query='query { currentUser { username } }'

  Specify query variables:

    	$ echo '<query>' | src api 'var1=val1' 'var2=val2'

  Searching for "Router" and getting result count:

    	$ echo 'query($query: String!) { search(query: $query) { results { resultCount } } }' | src api 'query=Router'

  Get the curl command for a query (just add '-get-curl' in the flags section):

    	$ src api -get-curl -query='query { currentUser { username } }'
`

var apiCommand = clicompat.Wrap(&cli.Command{
	Name:                      "api",
	Usage:                     "interacts with the Sourcegraph GraphQL API",
	UsageText:                 "src api [options] [variable=value ...]",
	Description:               apiExamples,
	HideVersion:               true,
	DisableSliceFlagSeparator: true,
	Flags: clicompat.WithAPIFlags(
		&cli.StringFlag{
			Name:  "query",
			Usage: "GraphQL query to execute, e.g. 'query { currentUser { username } }' (stdin otherwise)",
		},
		&cli.StringFlag{
			Name:  "vars",
			Usage: `GraphQL query variables to include as JSON string, e.g. '{"var": "val", "var2": "val2"}'`,
		},
	),
	Action: func(ctx context.Context, cmd *cli.Command) error {
		query := cmd.String("query")
		if query == "" {
			if isatty.IsTerminal(os.Stdin.Fd()) {
				return cmderrors.Usage("expected query to be piped into 'src api' or -query flag to be specified")
			}
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return err
			}
			query = string(data)
		}

		vars := map[string]any{}
		if raw := cmd.String("vars"); raw != "" {
			if err := json.Unmarshal([]byte(raw), &vars); err != nil {
				return err
			}
		}
		for _, arg := range cmd.Args().Slice() {
			key, value, ok := strings.Cut(arg, "=")
			if !ok {
				return cmderrors.Usagef("parsing argument %q expected 'variable=value' syntax (missing equals)", arg)
			}
			vars[key] = value
		}

		var result struct {
			Data   any               `json:"data,omitempty"`
			Errors []json.RawMessage `json:"errors,omitempty"`
		}
		client := cfg.apiClient(clicompat.APIFlagsFromCmd(cmd), cmd.Writer)
		if ok, err := client.NewRequest(query, vars).DoRaw(ctx, &result); err != nil || !ok {
			return err
		}

		formatted, err := marshalIndent(result)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(cmd.Writer, string(formatted))
		if err != nil {
			return err
		}
		if len(result.Errors) > 0 {
			return cmderrors.ExitCode(cmderrors.GraphqlErrorsExitCode, nil)
		}
		return nil
	},
})

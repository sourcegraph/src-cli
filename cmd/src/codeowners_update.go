package main

import (
	"context"
	"strings"

	"github.com/sourcegraph/sourcegraph/lib/errors"

	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/clicompat"
	"github.com/sourcegraph/src-cli/internal/cmderrors"
	"github.com/urfave/cli/v3"
)

const codeownersUpdateExamples = `
Update a codeowners file for a repository.

Examples:

	$ src codeowners update -repo='github.com/sourcegraph/sourcegraph' -f CODEOWNERS
	$ src codeowners update -repo='github.com/sourcegraph/sourcegraph' -f -
`

var codeownersUpdateCommand = clicompat.Wrap(&cli.Command{
	Name:        "update",
	Usage:       "update a codeowners file",
	UsageText:   "src codeowners update [options]",
	Description: codeownersUpdateExamples,
	HideVersion: true,
	Flags: clicompat.WithAPIFlags(
		&cli.StringFlag{
			Name:      "repo",
			Usage:     "The repository to attach the data to",
			Required:  true,
			Validator: requiresNotEmpty("provide a repo name using -repo"),
		},
		&cli.StringFlag{
			Name:      "file",
			Aliases:   []string{"f"},
			Usage:     "File path to read ownership information from (- for stdin)",
			TakesFile: true,
			Required:  true,
			Validator: requiresNotEmpty("provide a file using -file"),
		},
	),
	Action: func(ctx context.Context, cmd *cli.Command) error {
		repoName := cmd.String("repo")
		fileName := cmd.String("file")

		content, err := readFile(fileName)
		if err != nil {
			return err
		}

		client := cfg.apiClient(clicompat.APIFlagsFromCmd(cmd), cmd.Writer)

		query := `mutation UpdateCodeownersFile(
	$repoName: String!,
	$content: String!
) {
	updateCodeownersFile(input: {
		repoName: $repoName,
		fileContents: $content,
	}) {
		...CodeownersFileFields
	}
}
` + codeownersFragment

		var result struct {
			UpdateCodeownersFile CodeownersIngestedFile
		}
		if ok, err := client.NewRequest(query, map[string]any{
			"repoName": repoName,
			"content":  string(content),
		}).Do(ctx, &result); err != nil || !ok {
			var gqlErr api.GraphQlErrors
			if errors.As(err, &gqlErr) {
				for _, e := range gqlErr {
					if strings.Contains(e.Error(), "repo not found:") {
						return cmderrors.ExitCode(2, errors.Newf("repository %q not found", repoName))
					}
					if strings.Contains(e.Error(), "could not update codeowners file: codeowners file not found:") {
						return cmderrors.ExitCode(2, errors.New("no codeowners data has been found for this repository"))
					}
				}
			}
			return err
		}

		return nil
	},
})

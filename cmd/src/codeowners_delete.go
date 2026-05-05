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

const codeownersDeleteExamples = `
Delete a codeowners file for a repository.

Examples:

	$ src codeowners delete -repo='github.com/sourcegraph/sourcegraph'
`

var codeownersDeleteCommand = clicompat.Wrap(&cli.Command{
	Name:        "delete",
	Usage:       "delete a codeowners file",
	UsageText:   "src codeowners delete [options]",
	Description: codeownersDeleteExamples,
	HideVersion: true,
	Flags: clicompat.WithAPIFlags(
		&cli.StringFlag{
			Name:      "repo",
			Usage:     "The repository to delete the data for",
			Required:  true,
			Validator: requiresNotEmpty("provide a repo name using -repo"),
		},
	),
	Action: func(ctx context.Context, cmd *cli.Command) error {
		repoName := cmd.String("repo")
		client := cfg.apiClient(clicompat.APIFlagsFromCmd(cmd), cmd.Writer)

		query := `mutation DeleteCodeownersFile(
	$repoName: String!,
) {
	deleteCodeownersFiles(repositories: [{
		repoName: $repoName,
	}]) {
		alwaysNil
	}
}
`

		var result struct {
			DeleteCodeownersFile CodeownersIngestedFile
		}
		if ok, err := client.NewRequest(query, map[string]any{
			"repoName": repoName,
		}).Do(ctx, &result); err != nil || !ok {
			var gqlErr api.GraphQlErrors
			if errors.As(err, &gqlErr) {
				for _, e := range gqlErr {
					if strings.Contains(e.Error(), "repo not found:") {
						return cmderrors.ExitCode(2, errors.Newf("repository %q not found", repoName))
					}
					if strings.Contains(e.Error(), "codeowners file not found:") {
						return cmderrors.ExitCode(2, errors.Newf("no data found for repository %q", repoName))
					}
				}
			}
			return err
		}

		return nil
	},
})

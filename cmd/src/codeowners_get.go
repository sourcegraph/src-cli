package main

import (
	"context"
	"fmt"

	"github.com/sourcegraph/sourcegraph/lib/errors"

	"github.com/sourcegraph/src-cli/internal/clicompat"
	"github.com/sourcegraph/src-cli/internal/cmderrors"
	"github.com/urfave/cli/v3"
)

const codeownersGetExamples = `
Read the current codeowners file for a repository.

Examples:

	$ src codeowners get -repo='github.com/sourcegraph/sourcegraph'
`

var codeownersGetCommand = clicompat.Wrap(&cli.Command{
	Name:        "get",
	Usage:       "returns the codeowners file for a repository, if it exists",
	UsageText:   "src codeowners get [options]",
	Description: codeownersGetExamples,
	HideVersion: true,
	Flags: clicompat.WithAPIFlags(
		&cli.StringFlag{
			Name:      "repo",
			Usage:     "The repository to attach the data to",
			Required:  true,
			Validator: requiresNotEmpty("provide a repo name using -repo"),
		},
	),
	Action: func(ctx context.Context, cmd *cli.Command) error {
		repoName := cmd.String("repo")
		client := cfg.apiClient(clicompat.APIFlagsFromCmd(cmd), cmd.Writer)

		query := `query GetCodeownersFile(
	$repoName: String!
) {
	repository(name: $repoName) {
		ingestedCodeowners {
			...CodeownersFileFields
		}
	}
}
` + codeownersFragment

		var result struct {
			Repository *struct {
				IngestedCodeowners *CodeownersIngestedFile
			}
		}
		if ok, err := client.NewRequest(query, map[string]any{
			"repoName": repoName,
		}).Do(ctx, &result); err != nil || !ok {
			return err
		}

		if result.Repository == nil {
			return cmderrors.ExitCode(2, errors.Newf("repository %q not found", repoName))
		}

		if result.Repository.IngestedCodeowners == nil {
			return cmderrors.ExitCode(2, errors.Newf("no codeowners data found for %q", repoName))
		}

		_, err := fmt.Fprint(cmd.Writer, result.Repository.IngestedCodeowners.Contents)
		return err
	},
})

package main

import (
	"io"
	"os"

	"github.com/sourcegraph/sourcegraph/lib/errors"
	"github.com/sourcegraph/src-cli/internal/clicompat"
	"github.com/urfave/cli/v3"
)

const codeownersExamples = `'src codeowners' manages ingested code ownership data in a Sourcegraph instance.

Usage:

	src codeowners [command options]

Examples:

	$ src codeowners get -repo='github.com/sourcegraph/sourcegraph'
	$ src codeowners create -repo='github.com/sourcegraph/sourcegraph' -f CODEOWNERS
	$ src codeowners update -repo='github.com/sourcegraph/sourcegraph' -f CODEOWNERS
	$ src codeowners delete -repo='github.com/sourcegraph/sourcegraph'
`

const codeownersFragment = `
fragment CodeownersFileFields on CodeownersIngestedFile {
    contents
    repository {
		name
	}
}
`

type CodeownersIngestedFile struct {
	Contents   string `json:"contents"`
	Repository struct {
		Name string `json:"name"`
	} `json:"repository"`
}

var codeownersCommand = clicompat.Wrap(&cli.Command{
	Name:        "codeowners",
	Aliases:     []string{"codeowner"},
	Usage:       "manages ingested code ownership data",
	UsageText:   "src codeowners [command options]",
	Description: codeownersExamples,
	HideVersion: true,
	Commands: []*cli.Command{
		codeownersGetCommand,
		codeownersCreateCommand,
		codeownersUpdateCommand,
		codeownersDeleteCommand,
	},
})

func readFile(f string) ([]byte, error) {
	if f == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(f)
}

func requiresNotEmpty(errMsg string) func(string) error {
	return func(value string) error {
		if value == "" {
			return errors.New(errMsg)
		}
		return nil
	}
}

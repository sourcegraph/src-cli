package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/sourcegraph/sourcegraph/lib/errors"

	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/cmderrors"
)

func init() {
	usage := `
Examples:

  Read the current codeowners file for the repository "github.com/sourcegraph/sourcegraph":

    	$ src codeowners get -repo='github.com/sourcegraph/sourcegraph'
`

	flagSet := flag.NewFlagSet("get", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src codeowners %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}
	var (
		repoFlag = flagSet.String("repo", "", "The repository to attach the data to")
		apiFlags = api.NewFlags(flagSet)
	)

	handler := func(args []string) error {
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		if *repoFlag == "" {
			return errors.New("provide a repo name")
		}

		client := cfg.apiClient(apiFlags, flagSet.Output())

		query := `mutation GetCodeownersFile(
	$repoName: String!
) {
	repo(name: $repoName) {
		ingestedCodeownersFile {
			...CodeownersFileFields			
		}
	}
}
` + codeownersFragment

		var result struct {
			Repo *struct {
				IngestedCodeownersFile CodeownersIngestedFile
			}
		}
		if ok, err := client.NewRequest(query, map[string]interface{}{
			"repoName": *repoFlag,
		}).Do(context.Background(), &result); err != nil || !ok {
			return err
		}

		if result.Repo == nil {
			return cmderrors.ExitCode(2, errors.Newf("repository %q not found", *repoFlag))
		}

		fmt.Fprintf(os.Stdout, "%s", result.Repo.IngestedCodeownersFile.Contents)

		return nil
	}

	// Register the command.
	codeownersCommands = append(codeownersCommands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}

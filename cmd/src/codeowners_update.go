package main

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/sourcegraph/sourcegraph/lib/errors"

	"github.com/sourcegraph/src-cli/internal/api"
)

func init() {
	usage := `
Examples:

  Update a codeowners file for the repository "github.com/sourcegraph/sourcegraph":

    	$ src codeowners update -repo='github.com/sourcegraph/sourcegraph' -f CODEOWNERS

  Update a codeowners file for the repository "github.com/sourcegraph/sourcegraph" from stdin:

    	$ src codeowners update -repo='github.com/sourcegraph/sourcegraph' -f -
`

	flagSet := flag.NewFlagSet("update", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src codeowners %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}
	var (
		repoFlag = flagSet.String("repo", "", "The repository to attach the data to")
		fileFlag = flagSet.String("f", "", "File path to read ownership information from (- for stdin)")
		apiFlags = api.NewFlags(flagSet)
	)

	handler := func(args []string) error {
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		if *repoFlag == "" {
			return errors.New("provide a repo name")
		}

		if *fileFlag == "" {
			return errors.New("provide a file")
		}

		file, err := readFile(*fileFlag)
		if err != nil {
			return err
		}

		content, err := io.ReadAll(file)
		if err != nil {
			return err
		}

		client := cfg.apiClient(apiFlags, flagSet.Output())

		query := `mutation UpdateCodeownersFile(
	$repoName: String!,
	$content: String!
) {
	updateCodeownersFile(
		repoName: $repoName,
		fileContents: $content,
	) {
		...CodeownersFileFields
	}
}
` + codeownersFragment

		var result struct {
			UpdateCodeownersFile CodeownersIngestedFile
		}
		if ok, err := client.NewRequest(query, map[string]interface{}{
			"repoName": *repoFlag,
			"content":  string(content),
		}).Do(context.Background(), &result); err != nil || !ok {
			return err
		}

		return nil
	}

	// Register the command.
	codeownersCommands = append(codeownersCommands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}

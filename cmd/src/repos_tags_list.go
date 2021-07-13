package main

import (
	"context"
	"errors"
	"flag"

	"github.com/sourcegraph/src-cli/internal/api"
)

func init() {
	usage := `
Examples:

	List tags in a repository:

		$ src repos tags list github.com/sourcegraph/src-cli

`

	var (
		flagSet   = flag.NewFlagSet("list", flag.ExitOnError)
		firstFlag = flagSet.Int("first", 15, "Returns the first n tags from the list. (use -1 for unlimited)")
		apiFlags  = api.NewFlags(flagSet)
	)

	handler := func(args []string) error {
		ctx := context.Background()
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		if len(args) == 0 {
			return errors.New("at least one repository must be provided")
		}

		client := cfg.apiClient(apiFlags, flagSet.Output())

		for _, repo := range args {
			if err := showRepoTags(ctx, client, repo, *firstFlag, nil); err != nil {
				return err
			}
		}

		return nil
	}

	reposTagsCommands = append(reposTagsCommands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: makeReposTagsUsage(flagSet, usage),
	})
}

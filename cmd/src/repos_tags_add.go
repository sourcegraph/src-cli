package main

import (
	"context"
	"errors"
	"flag"
	"sync"

	"github.com/hashicorp/go-multierror"
	"github.com/sourcegraph/src-cli/internal/api"
)

func init() {
	usage := `
Examples:

	Add a tag to a repository:

		$ src repos tags add github.com/sourcegraph/src-cli cli

	Add multiple tags to a repository:
		
		$ src repos tags add github.com/sourcegraph/src-cli cli team/batchers

`
	var (
		flagSet  = flag.NewFlagSet("add", flag.ExitOnError)
		apiFlags = api.NewFlags(flagSet)
	)

	handler := func(args []string) error {
		ctx := context.Background()
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		if len(args) < 2 {
			return errors.New("at least one repository and one tag must be provided")
		}

		client := cfg.apiClient(apiFlags, flagSet.Output())

		mutation := `
			mutation AddRepoTag($repo: ID!, $tag: String!) {
                setTag(node: $repo, tag: $tag, present: true) {
                    alwaysNil
                }
			}
		`

		id, err := repoCache.Get(ctx, client, args[0])
		if err != nil {
			return err
		}

		var (
			wg    sync.WaitGroup
			errs  *multierror.Error
			errMu sync.Mutex
		)
		for _, tag := range args[1:] {
			wg.Add(1)
			go func(tag string) {
				defer wg.Done()

				if ok, err := client.NewRequest(mutation, map[string]interface{}{
					"repo": id,
					"tag":  tag,
				}).Do(ctx, &struct{}{}); err != nil || !ok {
					errMu.Lock()
					defer errMu.Unlock()

					errs = multierror.Append(errs, err)
				}
			}(tag)
		}
		wg.Wait()
		if err := errs.ErrorOrNil(); err != nil {
			return err
		}

		if err := showRepoTags(ctx, client, args[0], 9999, args[1:]); err != nil {
			return err
		}

		return nil
	}

	reposTagsCommands = append(reposTagsCommands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: makeReposTagsUsage(flagSet, usage),
	})
}

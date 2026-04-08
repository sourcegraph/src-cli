package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/sourcegraph/sourcegraph/lib/errors"

	"github.com/sourcegraph/src-cli/internal/api"
)

var reposCommands commander

func init() {
	usage := `'src repos' is a tool that manages repositories on a Sourcegraph instance.

Usage:

	src repos command [command options]

The commands are:

	get        		gets a repository
	list       		lists repositories
	delete 	   		deletes repositories
	add-metadata    adds a key-value pair metadata to a repository
	update-metadata updates a key-value pair metadata on a repository
	delete-metadata deletes a key-value pair metadata from a repository

Use "src repos [command] -h" for more information about a command.
`

	flagSet := flag.NewFlagSet("repos", flag.ExitOnError)
	handler := func(args []string) error {
		reposCommands.run(flagSet, "src repos", usage, args)
		return nil
	}

	// Register the command.
	commands = append(commands, &command{
		flagSet: flagSet,
		aliases: []string{"repo"},
		handler: handler,
		usageFunc: func() {
			fmt.Println(usage)
		},
	})
}

const repositoryFragment = `
fragment RepositoryFields on Repository {
	name
	defaultBranch {
		name
		displayName
	}
}
`

type Repository struct {
	Name          string `json:"name"`
	DefaultBranch GitRef `json:"defaultBranch"`
}

type GitRef struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
}

func fetchRepositoryID(ctx context.Context, client api.Client, repoName string) (string, error) {
	query := `query RepositoryID($repoName: String!) {
  repository(name: $repoName) {
    id
  }
}`

	var result struct {
		Repository struct {
			ID string
		}
	}
	if ok, err := client.NewRequest(query, map[string]any{
		"repoName": repoName,
	}).Do(ctx, &result); err != nil || !ok {
		return "", err
	}
	if result.Repository.ID == "" {
		return "", fmt.Errorf("repository not found: %s", repoName)
	}
	return result.Repository.ID, nil
}

func getRepoIdOrError(ctx context.Context, client api.Client, id *string, repoName *string) (*string, error) {
	if *id != "" {
		return id, nil
	} else if *repoName != "" {
		repoID, err := fetchRepositoryID(ctx, client, *repoName)
		return &repoID, err
	}
	return nil, errors.New("error: repo or repoName is required")
}

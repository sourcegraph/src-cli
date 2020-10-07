package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"text/template"

	"github.com/pkg/errors"
	"github.com/sourcegraph/src-cli/internal/campaigns"
)

func init() {
	usage := `
'src campaigns new' creates a new campaign spec YAML, prefilled with all
required fields.

Usage:

    src campaigns new [-f FILE]

Examples:


    $ src campaigns new -f campaign.spec.yaml

`

	flagSet := flag.NewFlagSet("new", flag.ExitOnError)

	var (
		fileFlag = flagSet.String("f", "campaign.yaml", "The name of campaign spec file to create.")
	)

	handler := func(args []string) error {
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		if _, err := os.Stat(*fileFlag); !os.IsNotExist(err) {
			return fmt.Errorf("file %s already exists", *fileFlag)
		}

		f, err := os.Create(*fileFlag)
		if err != nil {
			return errors.Wrapf(err, "failed to create file %s", *fileFlag)
		}
		defer f.Close()

		tmpl, err := template.New("").Parse(campaignSpecTmpl)
		if err != nil {
			return err
		}

		author := campaigns.GitCommitAuthor{
			Name:  "Sourcegraph",
			Email: "campaigns@sourcegraph.com",
		}

		// Try to get better default values from git, ignore any errors.
		if err := checkExecutable("git", "version"); err == nil {
			gitAuthorName, err := getGitConfig("user.name")
			if err == nil && gitAuthorName != "" {
				author.Name = gitAuthorName
			}

			gitAuthorEmail, err := getGitConfig("user.email")
			if err == nil && gitAuthorEmail != "" {
				author.Email = gitAuthorEmail
			}
		}

		return tmpl.Execute(f, map[string]interface{}{"Author": author})
	}

	campaignsCommands = append(campaignsCommands, &command{
		flagSet: flagSet,
		aliases: []string{},
		handler: handler,
		usageFunc: func() {
			fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src campaigns %s':\n", flagSet.Name())
			flagSet.PrintDefaults()
			fmt.Println(usage)
		},
	})
}

func getGitConfig(attribute string) (string, error) {
	cmd := exec.Command("git", "config", "--get", attribute)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

const campaignSpecTmpl = `name: hello-world-campaign
description: This campaign adds Hello World to READMEs

# Find all repositories that contain a README.md file.
on:
  - repositoriesMatchingQuery: file:README.md

# In each repository, run this command. Each repository's resulting diff is captured.
steps:
  - run: echo "Hello World" | tee -a $(find -name README.md)
    container: alpine:3

# Describe the changeset (e.g., GitHub pull request) you want for each repository.
changesetTemplate:
  title: Hello World
  body: This adds Hello World to the README
  branch: campaigns/hello-world # Push the commit to this branch.
  commit:
    author:
      name: {{ .Author.Name }}
      email: {{ .Author.Email }}
    message: Append Hello World to all README.md files

  # Change published to true once you're ready to create changesets on the code host.
  published: false
`

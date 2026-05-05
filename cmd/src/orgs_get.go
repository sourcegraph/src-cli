package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/clicompat"
	"github.com/urfave/cli/v3"
)

const orgsGetExamples = `
Examples:

  Get organization named abc-org:

    	$ src orgs get -name=abc-org

  List usernames of members of organization named abc-org (replace '.Username' with '.ID' to list user IDs):

    	$ src orgs get -f '{{range $i,$ := .Members.Nodes}}{{if ne $i 0}}{{"\n"}}{{end}}{{.Username}}{{end}}' -name=abc-org

`

var orgsGetCommand = clicompat.Wrap(&cli.Command{
	Name:        "get",
	Usage:       "gets an organization",
	UsageText:   "src orgs get [options]",
	Description: orgsGetExamples,
	HideVersion: true,
	Flags: clicompat.WithAPIFlags(
		&cli.StringFlag{
			Name:  "name",
			Usage: `Look up organization by name. (e.g. "abc-org")`,
		},
		&cli.StringFlag{
			Name:  "f",
			Value: "{{.|json}}",
			Usage: `Format for the output, using the syntax of Go package text/template. (e.g. "{{.ID}}: {{.Name}} ({{.DisplayName}})")`,
		},
	),
	Action: func(ctx context.Context, cmd *cli.Command) error {
		name := cmd.String("name")
		format := cmd.String("f")

		client := cfg.apiClient(clicompat.APIFlagsFromCmd(cmd), cmd.Writer)

		tmpl, err := parseTemplate(format)
		if err != nil {
			return err
		}

		query := `query Organization(
  $name: String!,
) {
  organization(
    name: $name
  ) {
    ...OrgFields
  }
}` + orgFragment

		var result struct {
			Organization *Org
		}
		if ok, err := client.NewRequest(query, map[string]any{
			"name": name,
		}).Do(ctx, &result); err != nil || !ok {
			return err
		}

		return execTemplate(tmpl, result.Organization)
	},
})

func init() {
	usage := orgsGetExamples

	flagSet := flag.NewFlagSet("get", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src orgs %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}
	var (
		nameFlag   = flagSet.String("name", "", `Look up organization by name. (e.g. "abc-org")`)
		formatFlag = flagSet.String("f", "{{.|json}}", `Format for the output, using the syntax of Go package text/template. (e.g. "{{.ID}}: {{.Name}} ({{.DisplayName}})")`)
		apiFlags   = api.NewFlags(flagSet)
	)

	handler := func(args []string) error {
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		client := cfg.apiClient(apiFlags, flagSet.Output())

		tmpl, err := parseTemplate(*formatFlag)
		if err != nil {
			return err
		}

		query := `query Organization(
  $name: String!,
) {
  organization(
    name: $name
  ) {
    ...OrgFields
  }
}` + orgFragment

		var result struct {
			Organization *Org
		}
		if ok, err := client.NewRequest(query, map[string]any{
			"name": *nameFlag,
		}).Do(context.Background(), &result); err != nil || !ok {
			return err
		}

		return execTemplate(tmpl, result.Organization)
	}

	// Register the command.
	orgsCommands = append(orgsCommands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}

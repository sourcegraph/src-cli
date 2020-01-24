package main

import (
	"flag"
	"fmt"

	"github.com/pkg/errors"
)

func init() {
	usage := `
Examples:

  Create a campaign with the given name, description and campaign plan:

    	$ src campaigns create -name="Format Go code"\
		   -desc="This Campaign runs gofmt over all Go repositories"\
		   -plan=Q2FtcGFpZ25QbGFuOjM=

`

	flagSet := flag.NewFlagSet("create", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src campaigns create %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}
	var (
		planIDFlag      = flagSet.String("plan", "", "ID of campaign plan the campaign should turn into changesets. (required)")
		nameFlag        = flagSet.String("name", "", "Name of the campaign. (required)")
		descriptionFlag = flagSet.String("desc", "", "Description for the campaign. (required)")
		formatFlag      = flagSet.String("f", "{{.ID}}: {{.Name}}", `Format for the output, using the syntax of Go package text/template. (e.g. "{{.ID}}: {{.Name}}") or "{{.|json}}")`)
		apiFlags        = newAPIFlags(flagSet)
	)

	handler := func(args []string) error {
		flagSet.Parse(args)

		if *planIDFlag == "" {
			return &usageError{errors.New("-plan must be specified")}
		}

		if *nameFlag == "" {
			return &usageError{errors.New("-name must be specified")}
		}

		tmpl, err := parseTemplate(*formatFlag)
		if err != nil {
			return err
		}

		var result struct {
			CreateCampaign struct {
				ID          string
				Name        string
				Description string
			}
		}

		input := map[string]interface{}{
			"name":        nullString(*nameFlag),
			"description": nullString(*descriptionFlag),
			"plan":        nullString(*planIDFlag),
		}
		return (&apiRequest{
			query: createCampaignQuery,
			vars: map[string]interface{}{
				"input": input,
			},
			result: &result,
			done: func() error {
				return execTemplate(tmpl, result.CreateCampaign)
			},
			flags: apiFlags,
		}).do()
	}

	// Register the command.
	campaignsCommands = append(campaignsCommands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}

const createCampaignQuery = `mutation CreateCampaign($input: CreateCampaignInput) {
  createCampaign(input: $input) {
    id
    name
    description
  }
}
`

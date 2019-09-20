package main

import (
	"flag"
	"fmt"
	"time"
)

func init() {
	usage := `
Examples:

  List campaigns (default limit is 1000):

    	$ src campaigns list

  List only the first 5 campaigns:

    	$ src campaigns list -first=5

  List campaigns and only print their IDs (default is to print ID and Name):

    	$ src campaigns list -first=5 -f '{{.ID}}'

`

	flagSet := flag.NewFlagSet("list", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src repos %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}
	var (
		firstFlag  = flagSet.Int("first", 1000, "Returns the first n repositories from the list. (use -1 for unlimited)")
		formatFlag = flagSet.String("f", "{{.ID}}: {{.Name}}", `Format for the output, using the syntax of Go package text/template. (e.g. "{{.ID}}: {{.Name}}") or "{{.|json}}")`)
		apiFlags   = newAPIFlags(flagSet)
	)

	handler := func(args []string) error {
		flagSet.Parse(args)

		tmpl, err := parseTemplate(*formatFlag)
		if err != nil {
			return err
		}

		query := `query Campaigns($first: Int) {
  campaigns(first: $first) {
    nodes {
      id
      name
      description
      createdAt
      updatedAt

      changesets {
        nodes {
          id
          state
          reviewState
          repository {
            id
            name
          }
          externalURL {
            url
            serviceType
          }
          createdAt
          updatedAt
        }
        totalCount
      }
    }
  }
}
`

		var result struct {
			Campaigns struct {
				Nodes []struct {
					ID          string
					Name        string
					Description string
					CreatedAt   time.Time
					UpdatedAt   time.Time
					Changesets  struct {
						Nodes []struct {
							ID          string
							State       string
							ReviewState string
							Repository  struct {
								ID   string
								Name string
							}
							ExternalURL struct {
								URL         string
								ServiceType string
							}
							CreatedAt time.Time
							UpdatedAt time.Time
						}
						TotalCount int
					}
				}
			}
		}

		return (&apiRequest{
			query:  query,
			vars:   map[string]interface{}{"first": nullInt(*firstFlag)},
			result: &result,
			done: func() error {
				for _, c := range result.Campaigns.Nodes {
					if err := execTemplate(tmpl, c); err != nil {
						return err
					}
				}
				return nil
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

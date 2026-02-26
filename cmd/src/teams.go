package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/features"
)

var teamsCommands commander

func init() {
	usage := `'src teams' is a tool that manages teams in a Sourcegraph instance.

Usage:

	src teams command [command options]

The commands are:

	list	lists teams
	create	create a team
	update	update a team
	delete	delete a team
	members	manage team members, use "src teams members [command] -h" for more information.

Use "src teams [command] -h" for more information about a command.
`

	flagSet := flag.NewFlagSet("teams", flag.ExitOnError)
	handler := func(args []string) error {
		if err := checkTeamsAvailability(); err != nil {
			return err
		}
		teamsCommands.run(flagSet, "src teams", usage, args)
		return nil
	}

	// Register the command.
	commands = append(commands, &command{
		flagSet: flagSet,
		aliases: []string{"team"},
		handler: handler,
		usageFunc: func() {
			fmt.Println(usage)
		},
	})
}

// checkTeamsAvailability verifies that the connected Sourcegraph instance
// supports teams. Teams were removed in Sourcegraph 7.0.
func checkTeamsAvailability() error {
	client := cfg.apiClient(api.NewFlags(flag.NewFlagSet("", flag.ContinueOnError)), os.Stderr)

	version, err := api.GetSourcegraphVersion(context.Background(), client)
	if err != nil || version == "" {
		// If we can't determine the version, let the command proceed.
		return nil
	}

	var ffs features.FeatureFlags
	if err := ffs.SetFromVersion(version, true); err != nil {
		return nil
	}
	if ffs.Sourcegraph70 {
		return fmt.Errorf("the 'src teams' commands are not available for Sourcegraph versions 7.0 and later (detected version: %s). Teams have been removed", version)
	}
	return nil
}

const teamFragment = `
fragment TeamFields on Team {
    id
    name
    displayName
	readonly
}
`

type Team struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Readonly    bool   `json:"readonly"`
}

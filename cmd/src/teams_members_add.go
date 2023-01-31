package main

import (
	"context"
	"flag"
	"fmt"
	"strings"

	"github.com/sourcegraph/sourcegraph/lib/errors"

	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/cmderrors"
)

func init() {
	usage := `
Examples:

  Add a team member:

    	$ src teams members add -team-name='engineering' [-email='alice@sourcegraph.com'] [-username='alice'] [-id='VXNlcjox'] [-external-account-service-id='https://github.com/' -external-account-service-type='github' [-external-account-account-id='123123123'] [-external-account-login='alice']]

`

	flagSet := flag.NewFlagSet("add", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src teams %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}
	var (
		teamNameFlag                   = flagSet.String("team-name", "", "The team name")
		emailFlag                      = flagSet.String("email", "", "Email to match the user by")
		usernameFlag                   = flagSet.String("username", "", "Optional name or ID of the parent team")
		idFlag                         = flagSet.String("id", "", "Optional name or ID of the parent team")
		externalAccountServiceIDFlag   = flagSet.String("external-account-service-id", "", "Optional name or ID of the parent team")
		externalAccountServiceTypeFlag = flagSet.String("external-account-service-type", "", "Optional name or ID of the parent team")
		externalAccountAccountIDFlag   = flagSet.String("external-account-account-id", "", "Optional name or ID of the parent team")
		externalAccountLoginFlag       = flagSet.String("external-account-login", "", "Optional name or ID of the parent team")
		apiFlags                       = api.NewFlags(flagSet)
	)

	handler := func(args []string) error {
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		if *teamNameFlag == "" {
			return errors.New("provide a team name")
		}

		client := cfg.apiClient(apiFlags, flagSet.Output())

		query := `mutation AddTeamMember(
	$teamName: String!
	$id: ID,
	$email: String,
	$username: String,
	$externalAccountServiceID: String,
	$externalAccountServiceType: String,
	$externalAccountAccountID: String,
	$externalAccountLogin: String,
) {
	addTeamMembers(
		teamName: $teamName,
		members: [{
			id: $id,
			email: $email,
			username: $username,
			externalAccountServiceID: $externalAccountServiceID,
			externalAccountServiceType: $externalAccountServiceType,
			externalAccountAccountID: $externalAccountAccountID,
			externalAccountLogin: $externalAccountLogin,
		}]
	) {
		...TeamFields
	}
}
` + teamFragment

		var result struct {
			AddTeamMembers Team
		}
		if ok, err := client.NewRequest(query, map[string]interface{}{
			"teamName":                   *teamNameFlag,
			"id":                         api.NullString(*idFlag),
			"email":                      api.NullString(*emailFlag),
			"username":                   api.NullString(*usernameFlag),
			"externalAccountServiceID":   api.NullString(*externalAccountServiceIDFlag),
			"externalAccountServiceType": api.NullString(*externalAccountServiceTypeFlag),
			"externalAccountAccountID":   api.NullString(*externalAccountAccountIDFlag),
			"externalAccountLogin":       api.NullString(*externalAccountLoginFlag),
		}).Do(context.Background(), &result); err != nil || !ok {
			var gqlErr api.GraphQlErrors
			if errors.As(err, &gqlErr) {
				for _, e := range gqlErr {
					// todo
					if strings.Contains(e.Error(), "team name is already taken") {
						return cmderrors.ExitCode(3, err)
					}
				}
			}
			return err
		}

		return nil
	}

	// Register the command.
	teamMembersCommands = append(teamMembersCommands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}

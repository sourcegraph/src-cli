package main

import (
	"context"

	"github.com/sourcegraph/sourcegraph/lib/errors"

	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/clicompat"
	"github.com/urfave/cli/v3"
)

const usersGetExamples = `
Examples:

  Get user with username alice:

    	$ src users get -username=alice

`

var usersGetCommand = clicompat.Wrap(&cli.Command{
	Name:        "get",
	Usage:       "gets a user",
	UsageText:   "src users get [options]",
	Description: usersGetExamples,
	HideVersion: true,
	Flags: clicompat.WithAPIFlags(
		&cli.StringFlag{
			Name:  "username",
			Usage: `Look up user by username. (e.g. "alice")`,
		},
		&cli.StringFlag{
			Name:  "email",
			Usage: `Look up user by email. (e.g. "alice@sourcegraph.com")`,
		},
		&cli.StringFlag{
			Name:  "f",
			Value: "{{.|json}}",
			Usage: `Format for the output, using the syntax of Go package text/template. (e.g. "{{.ID}}: {{.Username}} ({{.DisplayName}})")`,
		},
	),
	Action: func(ctx context.Context, cmd *cli.Command) error {
		username := cmd.String("username")
		email := cmd.String("email")
		format := cmd.String("f")

		client := cfg.apiClient(clicompat.APIFlagsFromCmd(cmd), cmd.Writer)

		if username != "" && email != "" {
			return errors.New("cannot specify both email and username")
		}

		tmpl, err := parseTemplate(format)
		if err != nil {
			return err
		}

		query := `query User(
  $username: String,
  $email: String,
) {
  user(
    username: $username,
    email: $email,
  ) {
    ...UserFields
  }
}` + userFragment

		var result struct {
			User *User
		}
		if ok, err := client.NewRequest(query, map[string]any{
			"username": api.NullString(username),
			"email":    api.NullString(email),
		}).Do(ctx, &result); err != nil || !ok {
			return err
		}

		return execTemplate(tmpl, result.User)
	},
})

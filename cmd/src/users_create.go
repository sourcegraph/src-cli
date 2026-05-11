package main

import (
	"context"
	"fmt"

	"github.com/sourcegraph/src-cli/internal/clicompat"
	"github.com/urfave/cli/v3"
)

const usersCreateExamples = `
Examples:

  Create a user account:

    	$ src users create -username=alice -email=alice@example.com

`

var usersCreateCommand = clicompat.Wrap(&cli.Command{
	Name:        "create",
	Usage:       "creates a user account",
	UsageText:   "src users create [options]",
	Description: usersCreateExamples,
	HideVersion: true,
	Flags: clicompat.WithAPIFlags(
		&cli.StringFlag{
			Name:      "username",
			Usage:     "The new user's username.",
			Required:  true,
			Validator: requiresNotEmpty("provide a username name using -username"),
		},
		&cli.StringFlag{
			Name:      "email",
			Usage:     "The new user's email address",
			Required:  true,
			Validator: requiresNotEmpty("provide a email name using -email"),
		},
		&cli.BoolFlag{
			Name:  "reset-password-url",
			Usage: "Print the reset password URL to manually send to the new user.",
		},
	),
	Action: func(ctx context.Context, cmd *cli.Command) error {
		username := cmd.String("username")
		email := cmd.String("email")
		resetPasswordURL := cmd.Bool("reset-password-url")

		client := cfg.apiClient(clicompat.APIFlagsFromCmd(cmd), cmd.Writer)

		query := `mutation CreateUser(
  $username: String!,
  $email: String!,
) {
  createUser(
    username: $username,
    email: $email,
  ) {
    resetPasswordURL
  }
}`

		var result struct {
			CreateUser struct {
				ResetPasswordURL string
			}
		}
		if ok, err := client.NewRequest(query, map[string]any{
			"username": username,
			"email":    email,
		}).Do(ctx, &result); err != nil || !ok {
			return err
		}

		if _, err := fmt.Fprintf(cmd.Writer, "User %q created.\n", username); err != nil {
			return err
		}
		if resetPasswordURL && result.CreateUser.ResetPasswordURL != "" {
			if _, err := fmt.Fprintln(cmd.Writer); err != nil {
				return err
			}
			_, err := fmt.Fprintf(cmd.Writer, "\tReset pasword URL: %s\n", result.CreateUser.ResetPasswordURL)
			return err
		}
		return nil
	},
})

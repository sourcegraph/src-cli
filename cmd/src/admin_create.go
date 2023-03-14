package main

import (
	"flag"
	"fmt"

	"github.com/sourcegraph/src-cli/internal/users"

	"github.com/sourcegraph/sourcegraph/lib/errors"
)

func init() {
	usage := `
Examples:

	Create an initial admin user on a new Sourcegraph deployment:

		$ src admin create -url https://your-sourcegraph-url -username admin -email admin@yourcompany.com -password p@55w0rd -with-token
`

	flagSet := flag.NewFlagSet("create", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src users %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}

	var (
		urlFlag      = flagSet.String("url", "", "The base URL for the Sourcegraph instance.")
		usernameFlag = flagSet.String("username", "", "The new admin user's username.")
		emailFlag    = flagSet.String("email", "", "The new admin user's email address.")
		passwordFlag = flagSet.String("password", "", "The new admin user's password.")
		tokenFlag    = flagSet.Bool("with-token", false, "Optionally create and output an admin access token.")
	)

	handler := func(args []string) error {
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		ok, _, err := users.NeedsSiteInit(*urlFlag)
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("failed to create admin, site already initialized")
		}

		client, err := users.SiteAdminInit(*urlFlag, *emailFlag, *usernameFlag, *passwordFlag)
		if err != nil {
			return err
		}

		if *tokenFlag {
			token, err := client.CreateAccessToken("", []string{"user:all", "site-admin:sudo"}, "src-cli")
			if err != nil {
				return err
			}

			_, err = fmt.Fprintf(flag.CommandLine.Output(), "%s", token)
			if err != nil {
				return err
			}
		}

		return nil
	}

	adminCommands = append(adminCommands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}

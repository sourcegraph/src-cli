package clicompat

import "github.com/urfave/cli/v3"

// Wrap sets common options on a sub commands to ensure consistency for help and error handling
func Wrap(cmd *cli.Command) *cli.Command {
	if cmd == nil {
		return nil
	}

	cmd.OnUsageError = OnUsageError
	return cmd
}

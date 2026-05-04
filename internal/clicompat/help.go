package clicompat

import (
	"context"
	"fmt"

	"github.com/sourcegraph/sourcegraph/lib/errors"
	"github.com/sourcegraph/src-cli/internal/cmderrors"
	"github.com/urfave/cli/v3"
)

// Wrap sets common options on a sub commands to ensure consistency for help and error handling
func Wrap(cmd *cli.Command) *cli.Command {
	if cmd == nil {
		return nil
	}

	cmd.OnUsageError = OnUsageError
	if cmd.Action == nil {
		cmd.Action = func(ctx context.Context, cmd *cli.Command) error {
			return cli.ShowSubcommandHelp(cmd)
		}
	} else {
		cmd.Action = wrapWithHelpOnUsageError(cmd.Action)
	}
	return cmd
}

func wrapWithHelpOnUsageError(action cli.ActionFunc) cli.ActionFunc {
	return func(ctx context.Context, cmd *cli.Command) error {
		err := action(ctx, cmd)
		if err != nil && errors.HasType[*cmderrors.UsageError](err) {
			_, _ = fmt.Fprintf(cmd.Root().ErrWriter, "error: %s\n---\n", err)
			cli.DefaultPrintHelp(cmd.Root().ErrWriter, cmd.CustomHelpTemplate, cmd)
		}
		return err
	}
}

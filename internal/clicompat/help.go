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
	cmd.Action = wrapUsageAction(cmd.Action)
	return cmd
}

func wrapUsageAction(action cli.ActionFunc) cli.ActionFunc {
	if action == nil {
		return nil
	}

	return func(ctx context.Context, cmd *cli.Command) error {
		err := action(ctx, cmd)
		if err == nil || !errors.HasType[*cmderrors.UsageError](err) {
			return err
		}

		_, _ = fmt.Fprintf(cmd.Root().ErrWriter, "error: %s\n", err)
		cli.DefaultPrintHelp(cmd.Root().ErrWriter, cmd.CustomHelpTemplate, cmd)
		return err
	}
}

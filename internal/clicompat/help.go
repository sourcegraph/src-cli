package clicompat

import (
	"context"
	"fmt"
	"io"
	"os"

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
	cmd.Action = wrapWithHelpOnUsageError(cmd.Action)
	return cmd
}

func wrapWithHelpOnUsageError(action cli.ActionFunc) cli.ActionFunc {
	if action == nil {
		return nil
	}

	return func(ctx context.Context, cmd *cli.Command) error {
		err := action(ctx, cmd)
		if err != nil && errors.HasType[*cmderrors.UsageError](err) {
			out := errWriter(cmd)
			_, _ = fmt.Fprintf(out, "error: %s\n---\n", err)
			printCommandHelp(out, cmd)
		}
		return err
	}
}

func errWriter(cmd *cli.Command) io.Writer {
	if cmd == nil || cmd.Root() == nil || cmd.Root().ErrWriter == nil {
		return os.Stderr
	}
	return cmd.Root().ErrWriter
}

func printCommandHelp(out io.Writer, cmd *cli.Command) {
	if cmd == nil {
		return
	}

	tmpl := cmd.CustomHelpTemplate
	if tmpl == "" {
		if len(cmd.Commands) == 0 {
			tmpl = cli.CommandHelpTemplate
		} else {
			tmpl = cli.SubcommandHelpTemplate
		}
	}

	cli.HelpPrinter(out, tmpl, cmd)
}

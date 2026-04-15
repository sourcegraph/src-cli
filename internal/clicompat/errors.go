package clicompat

import (
	"context"
	"fmt"
	"os"

	"github.com/sourcegraph/src-cli/internal/cmderrors"
	"github.com/urfave/cli/v3"
)

func OnUsageError(ctx context.Context, cmd *cli.Command, err error, isSubCommand bool) error {
	fmt.Fprintf(os.Stderr, "error: %s\n", err.Error())
	cli.DefaultPrintHelp(os.Stderr, cmd.CustomHelpTemplate, cmd)
	return cmderrors.Usage(err.Error())
}

package clicompat

import (
	"context"
	"fmt"

	"github.com/sourcegraph/src-cli/internal/cmderrors"
	"github.com/urfave/cli/v3"
)

func OnUsageError(ctx context.Context, cmd *cli.Command, err error, isSubCommand bool) error {
	out := errWriter(cmd)
	fmt.Fprintf(out, "error: %s\n", err.Error())
	printCommandHelp(out, cmd)
	return cmderrors.Usage(err.Error())
}

package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/sourcegraph/src-cli/internal/batches/ui"
	"github.com/sourcegraph/src-cli/internal/cmderrors"

	"github.com/sourcegraph/sourcegraph/lib/output"
)

func init() {
	usage := `
'src batch apply' is used to apply a batch spec on a Sourcegraph instance,
creating or updating the described batch change if necessary.

Usage:

    src batch apply [command options] [-f FILE]
    src batch apply [command options] FILE

Examples:

    $ src batch apply -f batch.spec.yaml

    $ src batch apply -f batch.spec.yaml -namespace myorg

    $ src batch apply batch.spec.yaml

`

	flagSet := flag.NewFlagSet("apply", flag.ExitOnError)
	flags := newBatchExecuteFlags(flagSet, false, batchDefaultCacheDir(), batchDefaultTempDirPrefix())

	handler := func(args []string) error {
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		if len(flagSet.Args()) != 0 {
			return cmderrors.Usage("additional arguments not allowed")
		}

		ctx, cancel := contextCancelOnInterrupt(context.Background())
		defer cancel()

		var execUI ui.ExecUI
		if flags.textOnly {
			execUI = &ui.JSONLines{}
		} else {
			out := output.NewOutput(flagSet.Output(), output.OutputOpts{Verbose: *verbose})
			execUI = &ui.TUI{Out: out}
		}

		err := executeBatchSpec(ctx, execUI, executeBatchSpecOpts{
			flags:  flags,
			client: cfg.apiClient(flags.api, flagSet.Output()),

			applyBatchSpec: true,
		})
		if err != nil {
			return cmderrors.ExitCode(1, nil)
		}

		return nil
	}

	batchCommands = append(batchCommands, &command{
		flagSet: flagSet,
		handler: handler,
		usageFunc: func() {
			fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src batch %s':\n", flagSet.Name())
			flagSet.PrintDefaults()
			fmt.Println(usage)
		},
	})
}

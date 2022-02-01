package main

import (
	"context"
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/sourcegraph/sourcegraph/lib/output"
	"github.com/sourcegraph/src-cli/internal/batches/service"
	"github.com/sourcegraph/src-cli/internal/batches/ui"
	"github.com/sourcegraph/src-cli/internal/cmderrors"
)

func init() {
	usage := `'src batch remote run' runs a batch spec on the Sourcegraph instance.

Usage:

    src batch remote run [-f FILE]

Examples:

    $ src batch remote run -f batch.spec.yaml

`

	flagSet := flag.NewFlagSet("run", flag.ExitOnError)
	flags := newBatchExecutionFlags(flagSet)

	var (
		fileFlag = flagSet.String("f", "batch.yaml", "The name of the batch spec file to create.")
	)

	handler := func(args []string) error {
		// Various bits of Batch Changes boilerplate.
		ctx := context.Background()

		if err := flagSet.Parse(args); err != nil {
			return err
		}

		if len(flagSet.Args()) != 0 {
			return cmderrors.Usage("additional arguments not allowed")
		}

		svc := service.New(&service.Opts{
			Client: cfg.apiClient(flags.api, flagSet.Output()),
		})

		if err := svc.DetermineFeatureFlags(ctx); err != nil {
			return err
		}

		out := output.NewOutput(flagSet.Output(), output.OutputOpts{Verbose: *verbose})
		ui := &ui.TUI{Out: out}

		// OK, now for the real stuff. We have to load in the batch spec, and we
		// may as well validate it at the same time so we don't even have to go to
		// the backend if it's invalid.
		ui.ParsingBatchSpec()
		_, raw, err := parseBatchSpec(fileFlag, svc)
		if err != nil {
			ui.ParsingBatchSpecFailure(err)
			return err
		}
		ui.ParsingBatchSpecSuccess()

		// We're going to need the namespace ID, so let's figure that out.
		ui.ResolvingNamespace()
		namespaceID, err := svc.ResolveNamespace(ctx, flags.namespace)
		if err != nil {
			return err
		}
		ui.ResolvingNamespaceSuccess(namespaceID)

		// TODO: add to ExecUI.
		pending := out.Pending(output.Line("", output.StylePending, "Sending batch spec"))
		batchSpecID, err := svc.UpsertBatchSpecInput(
			ctx,
			raw,
			namespaceID,
			flags.allowIgnored,
			flags.allowUnsupported,
			flags.clearCache,
		)
		if err != nil {
			pending.Complete(output.Linef(output.EmojiFailure, output.StyleWarning, "Error sending batch spec: %s", err.Error()))
			return err
		}
		pending.Complete(output.Line(output.EmojiSuccess, output.StyleSuccess, "Batch spec sent"))

		// Wait for the workspaces to be resolved.
		pending = out.Pending(output.Line("", output.StylePending, "Resolving workspaces"))
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			res, err := svc.GetBatchSpecWorkspaceResolution(ctx, batchSpecID)
			if err != nil {
				pending.Complete(output.Linef(output.EmojiFailure, output.StyleWarning, "Error resolving workspaces: %s", err.Error()))
				return err
			}

			if res.State == "FAILED" {
				pending.Complete(output.Linef(output.EmojiFailure, output.StyleWarning, "Workspace resolution failed: %s", res.FailureMessage))
				return errors.Newf("workspace resolution failed: %s", res.FailureMessage)
			} else if res.State == "COMPLETED" {
				pending.Complete(output.Line(output.EmojiSuccess, output.StyleSuccess, "Resolved workspaces"))
				break
			} else {
				pending.Updatef("Resolving workspaces: %s", res.State)
			}
		}

		// We have to enqueue this for execution with a separate operation.
		//
		// TODO: when the execute flag is wired up in the upsert mutation, just set
		// it there and remove this.
		pending = out.Pending(output.Line("", output.StylePending, "Executing on Sourcegraph"))
		batchSpecID, err = svc.ExecuteBatchSpec(ctx, batchSpecID, flags.clearCache)
		if err != nil {
			pending.Complete(output.Linef(output.EmojiFailure, output.StyleWarning, "Execution failed: %s", err.Error()))
			return err
		}

		// TODO: make beautiful, add a link, et cetera.
		pending.Complete(output.Linef(output.EmojiInfo, output.Fg256Color(12), "Executing at: %s/batch-changes/executions/%s", strings.TrimSuffix(cfg.Endpoint, "/"), batchSpecID))

		return nil
	}

	batchRemoteCommands = append(batchRemoteCommands, &command{
		flagSet: flagSet,
		aliases: []string{},
		handler: handler,
		usageFunc: func() {
			fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src batch remote %s':\n", flagSet.Name())
			flagSet.PrintDefaults()
			fmt.Println(usage)
		},
	})
}

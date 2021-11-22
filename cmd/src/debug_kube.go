package main

import (
	"flag"
	"fmt"
	"strings"

	"github.com/sourcegraph/src-cli/internal/cmderrors"
)

func init() {
	usage := `
'src debug kube' mocks kubectl commands to gather information about a kubernetes sourcegraph instance. 

Usage:

    src debug kube -o FILE [command options]

Examples:

    $ src debug kube -o debug.zip

`

	flagSet := flag.NewFlagSet("kube", flag.ExitOnError)
	var (
		base = flagSet.String("out", "debug.zip", "The name of the output zip archive")
	)

	handler := func(args []string) error {
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		//validate out flag
		if *base == "" {
			return fmt.Errorf("empty -out flag")
		}
		// declare basedir for archive file structure
		var baseDir string
		if strings.HasSuffix(*base, ".zip") == false {
			baseDir = *base
			*base = *base + ".zip"
		} else {
			baseDir = strings.TrimSuffix(*base, ".zip")
		}

		out, zw, ctx, err := setupDebug(*base)
		if err != nil {
			return fmt.Errorf("failed to open file: %w", err)
		}
		defer out.Close()
		defer zw.Close()

		err = archiveKube(ctx, zw, *verbose, baseDir)
		if err != nil {
			return cmderrors.ExitCode(1, nil)
		}
		return nil
	}

	debugCommands = append(batchCommands, &command{
		flagSet: flagSet,
		handler: handler,
		usageFunc: func() {
			fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src debug %s':\n", flagSet.Name())
			flagSet.PrintDefaults()
			fmt.Println(usage)
		},
	})
}

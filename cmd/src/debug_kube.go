package main

import (
	"flag"
	"fmt"
	"strings"

	"github.com/sourcegraph/src-cli/internal/cmderrors"
)

// TODO: seperate graphQL API call functions from archiveKube main function -- in accordance with database dumps, and prometheus, these should be optional via flags

func init() {
	usage := `
'src debug kube' mocks kubectl commands to gather information about a kubernetes sourcegraph instance. 

Usage:

    src debug kube -o FILE [command options]

Examples:

    $ src debug kube -o debug.zip

`

	flagSet := flag.NewFlagSet("kube", flag.ExitOnError)
	var base string
	var namespace string
	flagSet.StringVar(&base, "o", "debug.zip", "The name of the output zip archive")
	flagSet.StringVar(&namespace, "n", "default", "The namespace passed to kubectl commands, if not specified the default namespace is used")

	handler := func(args []string) error {
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		//validate out flag
		if base == "" {
			return fmt.Errorf("empty -o flag")
		}
		// declare basedir for archive file structure
		var baseDir string
		if strings.HasSuffix(base, ".zip") == false {
			baseDir = base
			base = base + ".zip"
		} else {
			baseDir = strings.TrimSuffix(base, ".zip")
		}

		out, zw, ctx, err := setupDebug(base)
		if err != nil {
			return err
		}
		defer out.Close()
		defer zw.Close()

		err = archiveKube(ctx, zw, *verbose, namespace, baseDir)
		if err != nil {
			return cmderrors.ExitCode(1, err)
		}
		return nil
	}

	debugCommands = append(debugCommands, &command{
		flagSet: flagSet,
		handler: handler,
		usageFunc: func() {
			fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src debug %s':\n", flagSet.Name())
			flagSet.PrintDefaults()
			fmt.Println(usage)
		},
	})
}

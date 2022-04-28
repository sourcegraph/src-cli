package main

import (
	"flag"
	"fmt"
	"strings"

	"github.com/sourcegraph/src-cli/internal/cmderrors"
)

func init() {
	usage := `
'src debug serv' mocks docker cli commands to gather information about a Sourcegraph server instance. 

Usage:

    src debug serv [command options]

Flags:

	-o			Specify the name of the output zip archive.
	-cfg		Include Sourcegraph configuration json. Defaults to true.

Examples:

    $ src debug serv -o debug.zip

	$ src -v debug serv -cfg=false -o foo.zip

`

	flagSet := flag.NewFlagSet("serv", flag.ExitOnError)
	var base string
	var configs bool
	flagSet.BoolVar(&configs, "cfg", true, "If true include Sourcegraph configuration files. Default value true.")
	flagSet.StringVar(&base, "o", "debug.zip", "The name of the output zip archive")

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
		if !strings.HasSuffix(base, ".zip") {
			baseDir = base
			base = base + ".zip"
		} else {
			baseDir = strings.TrimSuffix(base, ".zip")
		}

		out, zw, ctx, err := setupDebug(base)
		if err != nil {
			return fmt.Errorf("failed to open file: %w", err)
		}
		defer out.Close()
		defer zw.Close()

		err = archiveDocker(ctx, zw, *verbose, configs, baseDir)
		if err != nil {
			return cmderrors.ExitCode(1, nil)
		}
		return nil
	}

	debugCommands = append(debugCommands, &command{
		flagSet: flagSet,
		handler: handler,
		usageFunc: func() {
			fmt.Println(usage)
		},
	})
}

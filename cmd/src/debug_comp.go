package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strings"
	"unicode"

	"github.com/sourcegraph/src-cli/internal/cmderrors"
)

func init() {
	usage := `
'src debug comp' mocks docker cli commands to gather information about a docker-compose Sourcegraph instance.

Usage:

    src debug comp [command options]

Flags:

	-o			Specify the name of the output zip archive.
	-cfg		Include Sourcegraph configuration json. Defaults to true.

Examples:

    $ src debug comp -o debug.zip

	$ src -v debug comp -cfg=false -o foo.zip

`

	flagSet := flag.NewFlagSet("comp", flag.ExitOnError)
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
		if strings.HasSuffix(base, ".zip") == false {
			baseDir = base
			base = base + ".zip"
		} else {
			baseDir = strings.TrimSuffix(base, ".zip")
		}

		ctx := context.Background()
		containers, err := getContainers(ctx)

		log.Printf("Archiving docker-cli data for %d containers\n SRC_ENDPOINT: %v\n Output filename: %v", len(containers), cfg.Endpoint, base)

		var verify string
		fmt.Print("Do you want to start writing to an archive? [y/n] ")
		_, err = fmt.Scanln(&verify)
		for unicode.ToLower(rune(verify[0])) != 'y' && unicode.ToLower(rune(verify[0])) != 'n' {
			fmt.Println("Input must be string y or n")
			_, err = fmt.Scanln(&verify)
		}
		if unicode.ToLower(rune(verify[0])) == 'n' {
			fmt.Println("escaping")
			return nil
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

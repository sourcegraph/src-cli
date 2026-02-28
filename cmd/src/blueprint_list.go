package main

import (
	"context"
	"flag"
	"fmt"
	"path/filepath"

	"github.com/sourcegraph/src-cli/internal/blueprint"
)

func init() {
	usage := `
Examples:

  List blueprints from the default community repository:

    	$ src blueprint list

  List blueprints from a GitHub repository:

    	$ src blueprint list -repo https://github.com/org/blueprints

  List blueprints from a specific branch or tag:

    	$ src blueprint list -repo https://github.com/org/blueprints -rev v1.0.0

  List blueprints from a local directory:

    	$ src blueprint list -repo ./my-blueprints

  Print JSON description of all blueprints:

    	$ src blueprint list -f '{{.|json}}'

  List just blueprint names and subdirs:

    	$ src blueprint list -f '{{.Subdir}}: {{.Name}}'

`

	flagSet := flag.NewFlagSet("list", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src blueprint %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}

	var (
		repoFlag   = flagSet.String("repo", defaultBlueprintRepo, "Repository URL (HTTPS) or local path to blueprints")
		revFlag    = flagSet.String("rev", "", "Git revision, branch, or tag to checkout (ignored for local paths)")
		formatFlag = flagSet.String("f", "{{.Title}}\t{{.Summary}}\t{{.Subdir}}", `Format for the output, using the syntax of Go package text/template. (e.g. "{{.|json}}")`)
	)

	handler := func(args []string) error {
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		tmpl, err := parseTemplate(*formatFlag)
		if err != nil {
			return err
		}

		src, err := blueprint.ResolveRootSource(*repoFlag, *revFlag)
		if err != nil {
			return err
		}

		rootDir, cleanup, err := src.Prepare(context.Background())
		if cleanup != nil {
			defer func() { _ = cleanup() }()
		}
		if err != nil {
			return err
		}

		found, err := blueprint.FindBlueprints(rootDir)
		if err != nil {
			return err
		}

		for _, bp := range found {
			subdir, _ := filepath.Rel(rootDir, bp.Dir)
			if subdir == "." {
				subdir = ""
			}
			data := struct {
				*blueprint.Blueprint
				Subdir string
			}{bp, subdir}
			if err := execTemplate(tmpl, data); err != nil {
				return err
			}
		}
		return nil
	}

	blueprintCommands = append(blueprintCommands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}

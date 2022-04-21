package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strings"
	"unicode"

	"github.com/sourcegraph/src-cli/internal/cmderrors"
	"github.com/sourcegraph/src-cli/internal/exec"
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
	var extsvc bool
	flagSet.StringVar(&base, "o", "debug.zip", "The name of the output zip archive")
	flagSet.StringVar(&namespace, "n", "default", "The namespace passed to kubectl commands, if not specified the default namespace is used")
	flagSet.BoolVar(&extsvc, "ext", false, "Include external service json in archive")

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

		pods, err := getPods(ctx, namespace)
		if err != nil {
			return fmt.Errorf("failed to get pods: %w", err)
		}
		kubectx, err := exec.CommandContext(ctx, "kubectl", "config", "current-context").CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to get current-context: %w", err)
		}
		//TODO: improve formating to include ls like pod listing for pods targeted.
		log.Printf("getting kubectl data for %d pods, from context %s ...\n", len(pods.Items), kubectx)

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
			return err
		}
		defer out.Close()
		defer zw.Close()

		err = archiveKube(ctx, zw, *verbose, extsvc, namespace, baseDir, pods)
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

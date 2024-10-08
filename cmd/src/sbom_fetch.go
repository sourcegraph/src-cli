package main

import (
	"flag"
	"fmt"
	"os/exec"

	"github.com/sourcegraph/sourcegraph/lib/errors"
	"github.com/sourcegraph/sourcegraph/lib/output"

	"github.com/sourcegraph/src-cli/internal/cmderrors"
)

func init() {
	usage := `
'src sbom fetch' fetches and verifies SBOMs for the given release version of Sourcegraph.

Usage:

    src sbom fetch -v <version>

Examples:

    $ src sbom fetch 5.8.0
`

	flagSet := flag.NewFlagSet("fetch", flag.ExitOnError)
	versionFlag := flagSet.String("v", "", "The version of Sourcegraph to fetch SBOMs for.")

	handler := func(args []string) error {
		// ctx := context.Background()

		if err := flagSet.Parse(args); err != nil {
			return err
		}

		if len(flagSet.Args()) != 0 {
			return cmderrors.Usage("additional arguments not allowed")
		}

		if versionFlag == nil || *versionFlag == "" {
			return cmderrors.Usage("version is required")
		}

		out := output.NewOutput(flagSet.Output(), output.OutputOpts{Verbose: *verbose})
		// ui := &ui.TUI{Out: out}

		fmt.Printf("Version is %s\n", *versionFlag)

		out.WriteLine(output.Line("\u2705", output.StyleSuccess, "Doing some sbom stuff!."))

		if err := verifyCosign(); err != nil {
			return cmderrors.ExitCode(1, err)
		}

		images, err := getImageList()
		if err != nil {
			return err
		}

		fmt.Printf("Verifying SBOM for images: %v\n", images)

		return nil
	}

	sbomCommands = append(sbomCommands, &command{
		flagSet: flagSet,
		handler: handler,
		usageFunc: func() {
			fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src sbom %s':\n", flagSet.Name())
			flagSet.PrintDefaults()
			fmt.Println(usage)
		},
	})
}

func verifyCosign() error {
	_, err := exec.LookPath("cosign")
	if err != nil {
		return errors.New("SBOM verification requires 'cosign' to be installed and available in $PATH. See https://docs.sigstore.dev/cosign/system_config/installation/")
	}
	return nil
}

func getImageList() ([]string, error) {
	return []string{"gitserver", "frontend", "worker"}, nil
}

package main

import (
	"archive/zip"
	"flag"
	"fmt"
	"os"

	"github.com/sourcegraph/src-cli/internal/exec"
)

func init() {
	flagSet := flag.NewFlagSet("debug", flag.ExitOnError)

	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), `'src debug' gathers and bundles debug data from a Sourcegraph deployment.

USAGE
  src [-v] debug [-out=debug.zip]
`)
	}

	var (
		outFile = flagSet.String("out", "debug.zip", "The name of the output zip archive")
	)

	handler := func(args []string) error {
		err := flagSet.Parse(args)
		if err != nil {
			return err
		}

		if *outFile == "" {
			return fmt.Errorf("empty -out flag")
		}

		out, err := os.OpenFile(*outFile, os.O_CREATE|os.O_RDWR, 0666)
		if err != nil {
			return fmt.Errorf("failed to open file: %w", err)
		}

		defer out.Close()
		zw := zip.NewWriter(out)
		defer zw.Close()

		k8sEvents, err := zw.Create("k8s-events.txt")
		if err != nil {
			return fmt.Errorf("failed to create k8s-events.txt: %w", err)
		}

		cmd := exec.Command("kubectl", "get", "events")
		cmd.Stdout = k8sEvents
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("running kubectl get events failed: %w", err)
		}

		return nil
	}

	// Register the command.
	commands = append(commands, &command{
		aliases:   []string{"debug-dump"},
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}

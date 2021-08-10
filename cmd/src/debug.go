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

		// TODO: ensures outFile ends in .zip
		// if *outFile == !strings.HasSuffix(".zip") {
		//		return fmt.Errorf("-out flg must end in .zip")
		// }

		out, err := os.OpenFile(*outFile, os.O_CREATE|os.O_RDWR|os.O_EXCL, 0666)
		if err != nil {
			return fmt.Errorf("failed to open file: %w", err)
		}

		defer out.Close()
		zw := zip.NewWriter(out)
		defer zw.Close()

		// TODO: make outFile the input from `-out=` flag stored in `outFile`
		k8sEvents, err := zw.Create("outFile/kubectl/events.txt")
		if err != nil {
			return fmt.Errorf("failed to create k8s-events.txt: %w", err)
		}

		cmd := exec.Command("kubectl", "get", "events", "--all-namespaces")
		cmd.Stdout = k8sEvents
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("running kubectl get events failed: %w", err)
		}

		// Store logs output in zip
		k8sPods, err := zw.Create("outFile/kubectl/pods.txt")
		if err != nil {
			return fmt.Errorf("failed to create pods.txt: %w", err)
		}

		// TODO process pods output into array of strings to run in loop as argument for writing as .txt in archive
		// Get all pod names
		zmd := exec.Command("kubectl", "get", "pods")
		zmd.Stdout = k8sPods
		zmd.Stderr = os.Stderr
		fmt.Println(k8sPods)

		if err := zmd.Run(); err != nil {
			return fmt.Errorf("running kubectl get pods failed: %w", err)
		}

		// Sanity Check
		fmt.Println("runnnnnnnnn")

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

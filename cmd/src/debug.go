package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
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

		// TODO: make outFile the input from `-out=` flag stored in `outFile`, validate the .zip postpends `outFile`
		//write events to archive
		k8sEvents, err := zw.Create("outFile/kubectl/events.txt")
		if err != nil {
			return fmt.Errorf("failed to create k8s-events.txt: %w", err)
		}

		//define command to get events
		cmd := exec.Command("kubectl", "get", "events", "--all-namespaces")
		cmd.Stdout = k8sEvents
		cmd.Stderr = os.Stderr

		//get events
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("running kubectl get events failed: %w", err)
		}

		return savek8sLogs(zw)
	}

	// Register the command.
	commands = append(commands, &command{
		aliases:   []string{"debug-dump"},
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}

func savek8sLogs(zw *zip.Writer) error {
	var podsBuff bytes.Buffer

	// TODO process pods output into array of strings to run in loop as argument for writing as .txt in archive
	// parse json output with json.decode to create slice of pod names
	// Get all pod names
	getPods := exec.Command("kubectl", "get", "pods", "-l", "deploy=sourcegraph", "-o=json")
	//Output pointer to byte buffer
	getPods.Stdout = &podsBuff
	getPods.Stderr = os.Stderr

	if err := getPods.Run(); err != nil {
		return fmt.Errorf("running kubectl get pods failed: %w", err)
	}

	var podList struct {
		Items []struct {
			Metadata struct {
				Name string
			}
			Spec struct {
				Containers []struct {
					Name string
				}
			}
		}
	}

	err := json.NewDecoder(&podsBuff).Decode(&podList)
	if err != nil {
		return fmt.Errorf("failed to unmarshall pods json: %w", err)
	}

	// TODO: fix pods with sub containers
	// exec kubectl get logs and write to archive
	for _, pod := range podList.Items {
		fmt.Printf("%+v\n", pod)
		for _, container := range pod.Spec.Containers {
			logs, err := zw.Create("outFile/logs/" + pod.Metadata.Name + "/" + container.Name + ".txt")
			if err != nil {
				return fmt.Errorf("failed to create podLogs.txt: %w", err)
			}

			getLogs := exec.Command("kubectl", "logs", pod.Metadata.Name, "-c", container.Name)
			getLogs.Stdout = logs
			getLogs.Stderr = os.Stderr

			if err := getLogs.Run(); err != nil {
				return fmt.Errorf("running kubectl get logs failed: %w", err)
			}
		}
	}

	return nil
}

package main

import (
	"archive/zip"
	"bytes"
	"flag"
	"fmt"
	"os"
	"strings"

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

		var podsBuff bytes.Buffer

		// TODO process pods output into array of strings to run in loop as argument for writing as .txt in archive
		// parse json output with json.decode to create slice of pod names
		// Get all pod names
		getPods := exec.Command("kubectl", "get", "pods", "-o=jsonpath='{.items[*].metadata.name}'")
		//Output pointer to byte buffer
		getPods.Stdout = &podsBuff
		getPods.Stderr = os.Stderr

		if err := getPods.Run(); err != nil {
			return fmt.Errorf("running kubectl get pods failed: %w", err)
		}

		//convert output to slice
		pods := podsBuff.String()
		podList := strings.Split(pods, " ")
		//handle "'" in podList[0] and podList[len(podList) - 1)
		podList[0] = strings.TrimLeft(podList[0], "'")
		podList[len(podList)-1] = strings.TrimRight(podList[len(podList)-1], "'")
		//fmt.Println("right: ", podList[len(podList)-1], "left: ", podList[0])

		// TODO: fix pods with sub containers
		// exec kubectl get logs and write to archive
		for i := range podList {
			fmt.Println(podList[i])
			podLogs, err := zw.Create("outFile/logs/" + podList[i])
			if err != nil {
				return fmt.Errorf("failed to create podLogs.txt: %w", err)
			}

			getLogs := exec.Command("kubectl", "logs", podList[i])
			getLogs.Stdout = podLogs
			getLogs.Stderr = os.Stderr

			if err := getLogs.Run(); err != nil {
				return fmt.Errorf("running kubectl get logs failed: %w", err)
			}
		}

		// Store logs output in zip
		//k8sPods, err := zw.Create("outFile/kubectl/pods.txt")
		//if err != nil {
		//	return fmt.Errorf("failed to create pods.txt: %w", err)
		//}

		//fmt.Println(len(podList))
		//fmt.Println(podList[0])
		//fmt.Printf("%T \n\n", podsBuff)
		//fmt.Printf("%s \n\n", pods)
		//fmt.Printf("%T \n\n", pods)
		//fmt.Printf("%s \n\n", podList)
		//fmt.Printf("%T \n\n", podList)

		// Sanity Check
		//fmt.Println(" zmd rannnnnnnnn")

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

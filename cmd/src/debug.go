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

// Init debug flag on src build
func init() {
	flagSet := flag.NewFlagSet("debug", flag.ExitOnError)

	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), `'src debug' gathers and bundles debug data from a Sourcegraph deployment.

USAGE
  src [-v] debug [-out=debug.zip] 
`)
	}

	// store value passed to out flag
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

	// Get all pod names as json
	getPods := exec.Command("kubectl", "get", "pods", "-l", "deploy=sourcegraph", "-o=json")
	//Output pointer to byte buffer
	getPods.Stdout = &podsBuff
	getPods.Stderr = os.Stderr

	// Run getPods
	if err := getPods.Run(); err != nil {
		return fmt.Errorf("running kubectl get pods failed: %w", err)
	}

	//Declare struct to format decode from podList
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

	//Decode json from podList
	err := json.NewDecoder(&podsBuff).Decode(&podList)
	if err != nil {
		return fmt.Errorf("failed to unmarshall pods json: %w", err)
	}

	// exec kubectl get logs and write to archive, accounts for containers in pod
	for _, pod := range podList.Items {
		fmt.Println(pod.Metadata.Name, "containers:", pod.Spec.Containers)
		for _, container := range pod.Spec.Containers {
			logs, err := zw.Create("outFile/kubectl/logs/" + pod.Metadata.Name + "/" + container.Name + ".txt")
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

	// TODO: dont write a prev-container.txt if kubectl logs --previous returns no prev pod
	for _, pod := range podList.Items {
		fmt.Println(pod.Metadata.Name, "containers:", pod.Spec.Containers)
		for _, container := range pod.Spec.Containers {
			prevLogs, err := zw.Create("outFile/kubectl/logs/" + pod.Metadata.Name + "/" + "prev-" + container.Name + ".txt")
			if err != nil {
				return fmt.Errorf("failed to create podLogs.txt: %w", err)
			}

			getPrevLogs := exec.Command("kubectl", "logs", "--previous", pod.Metadata.Name, "-c", container.Name)
			getPrevLogs.Stderr = os.Stderr
			getPrevLogs.Stdout = prevLogs

			if err := getPrevLogs.Run(); err != nil {
				fmt.Errorf("running kubectl get logs failed: %w", err)
				// continue
			} else {
				fmt.Println("\nTHERE WAS A PREV!!!!")
			}
		}
	}

	return nil
}

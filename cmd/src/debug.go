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

// package level
type podList struct {
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
		// Declare buffer type var for kubectl pipe
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
		var pods podList

		//Decode json from podList
		if err := json.NewDecoder(&podsBuff).Decode(&pods); err != nil {
			return fmt.Errorf("failed to unmarshall pods json: %w", err)
		}
		fmt.Println(pods)

		archiveEvents(zw)
		archiveLogs(zw, pods)
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

//func getPods() error {
//	// Declare buffer type var for kubectl pipe
//	var podsBuff bytes.Buffer
//
//	// Get all pod names as json
//	getPods := exec.Command("kubectl", "get", "pods", "-l", "deploy=sourcegraph", "-o=json")
//	//Output pointer to byte buffer
//	getPods.Stdout = &podsBuff
//	getPods.Stderr = os.Stderr
//
//	// Run getPods
//	if err := getPods.Run(); err != nil {
//		return fmt.Errorf("running kubectl get pods failed: %w", err)
//	}
//
//	//Decode json from podList
//	err := json.NewDecoder(&podsBuff).Decode(&podList)
//	if err != nil {
//		return fmt.Errorf("failed to unmarshall pods json: %w", err)
//	}
//	return podList
//}

func archiveEvents(zw *zip.Writer) error {
	//write events to archive
	k8sEvents, err := zw.Create("outFile/kubectl/events.txt")
	if err != nil {
		return fmt.Errorf("failed to create k8s-events.txt: %w", err)
	}

	//define command to get events
	getEvents := exec.Command("kubectl", "get", "events", "--all-namespaces")
	getEvents.Stdout = k8sEvents
	getEvents.Stderr = os.Stderr

	//get events
	if err := getEvents.Run(); err != nil {
		return fmt.Errorf("running kubectl get events failed: %w", err)
	}
	return nil
}

func archiveLogs(zw *zip.Writer, pods podList) error {

	// run kubectl logs and write to archive, accounts for containers in pod
	for _, pod := range pods.Items {
		fmt.Println("Archiving logs: ", pod.Metadata.Name, "Containers:", pod.Spec.Containers)
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

	// run kubectl logs --previous and write to archive if return not err
	for _, pod := range pods.Items {
		for _, container := range pod.Spec.Containers {
			getPrevLogs := exec.Command("kubectl", "logs", "--previous", pod.Metadata.Name, "-c", container.Name)
			if err := getPrevLogs.Run(); err == nil {
				fmt.Println("Archiving previous logs: ", pod.Metadata.Name, "Containers: ", pod.Spec.Containers)
				prev, err := zw.Create("outFile/kubectl/logs/" + pod.Metadata.Name + "/" + "prev-" + container.Name + ".txt")
				getPrevLogs.Stdout = prev
				if err != nil {
					return fmt.Errorf("failed to create podLogs.txt: %w", err)
				}
			}
		}
	}

	return nil
}

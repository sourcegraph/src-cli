package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/sourcegraph/src-cli/internal/exec"
)

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

type archiveFile struct {
	name string
	data []byte
	err  error
}

// Init debug flag on src build
func init() {
	flagSet := flag.NewFlagSet("debug", flag.ExitOnError)

	usageFunc := func() {

		fmt.Fprintf(flag.CommandLine.Output(), `'src debug' gathers and bundles debug data from a Sourcegraph deployment.

USAGE
  src [-v] debug -d=<deployment type> [-out=debug.zip]
`)
	}

	// store value passed to flags
	var (
		deployment = flagSet.String("d", "", "deployment type")
		base       = flagSet.String("out", "debug.zip", "The name of the output zip archive")
	)

	handler := func(args []string) error {
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		//validate out flag
		if *base == "" {
			return fmt.Errorf("empty -out flag")
		}
		// declare basedir for archive file structure
		var baseDir string
		if strings.HasSuffix(*base, ".zip") == false {
			baseDir = *base
			*base = *base + ".zip"
		} else {
			baseDir = strings.TrimSuffix(*base, ".zip")
		}
		//// handle deployment flag
		//if !((*deployment == "serv") || (*deployment == "comp") || (*deployment == "kube")) {
		//	return fmt.Errorf("must declare -d=<deployment type>, as serv, comp, or kube")
		//}

		// open pipe to output file
		out, err := os.OpenFile(*base, os.O_CREATE|os.O_RDWR|os.O_EXCL, 0666)
		if err != nil {
			return fmt.Errorf("failed to open file: %w", err)
		}

		// open zip writer
		defer out.Close()
		zw := zip.NewWriter(out)
		defer zw.Close()

		// TODO write functions for sourcegraph server and docker-compose instances
		switch *deployment {
		case "serv":
			getContainers()
		case "comp":
			getContainers()
		case "kube":
			if err := archiveKube(zw, baseDir); err != nil {
				return fmt.Errorf("archiveKube failed with err: %w", err)
			}
		default:
			return fmt.Errorf("must declare -d=<deployment type>, as server, compose, or kubernetes")
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

/*
Kubernetes functions
TODO: improve logging as kubectl calls run (Desc, Mani)
TODO: refactor archiveLLogs so that both logs and logs --past are collected in the same loop
TODO: refactor archive functions to run concurrently as goroutines
*/

// Run kubectl functions concurrently and archive results to zip file
func archiveKube(zw *zip.Writer, baseDir string) error {
	pods, err := getPods()
	if err != nil {
		return fmt.Errorf("failed to get pods: %w", err)
	}
	fmt.Println(pods)

	// setup channel for slice of archive function outputs
	ch := make(chan []archiveFile)
	wg := sync.WaitGroup{}

	// create goroutine to get kubectl events
	wg.Add(1)
	go func() {
		defer wg.Done()
		fs := getEvents(baseDir)
		ch <- fs
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		fs := getPV(baseDir)
		ch <- fs
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		fs := getPVC(baseDir)
		ch <- fs
	}()

	// close channel when wait group goroutines have completed
	go func() {
		wg.Wait()
		close(ch)
	}()

	// write to archive all the outputs from kubectl call functions passed to buffer channel
	for files := range ch {
		for _, f := range files {
			// write file path
			zf, err := zw.Create(f.name)
			if err != nil {
				return fmt.Errorf("failed to create %s: %w", f.name, err)
			}

			_, err = zf.Write(f.data)
			if err != nil {
				return fmt.Errorf("failed to write to %s: %w", f.name, err)
			}
		}
	}

	return nil
}

func getPods() (podList, error) {
	// Declare buffer type var for kubectl pipe
	var podsBuff bytes.Buffer

	// Get all pod names as json
	getPods := exec.Command("kubectl", "get", "pods", "-l", "deploy=sourcegraph", "-o=json")
	getPods.Stdout = &podsBuff
	getPods.Stderr = os.Stderr
	err := getPods.Run()

	//Declare struct to format decode from podList
	var pods podList

	//Decode json from podList
	if err := json.NewDecoder(&podsBuff).Decode(&pods); err != nil {
		fmt.Errorf("feailed to unmarshall get pods json: %w", err)
	}

	fmt.Println(pods)
	return pods, err
}

func getEvents(baseDir string) (fs []archiveFile) {
	f := archiveFile{name: baseDir + "/kubectl/events.txt"}

	f.data, f.err = exec.Command("kubectl", "get", "events", "--all-namespaces").CombinedOutput()

	return []archiveFile{f}
}

func getPV(baseDir string) (fs []archiveFile) {
	f := archiveFile{name: baseDir + "/kubectl/persistent-volumes.txt"}

	f.data, f.err = exec.Command("kubectl", "get", "pv").CombinedOutput()

	return []archiveFile{f}
}

func getPVC(baseDir string) (fs []archiveFile) {
	//write persistent volume claims to archive
	f := archiveFile{name: baseDir + "/kubectl/persistent-volume-claims.txt"}

	f.data, f.err = exec.Command("kubectl", "get", "pvc").CombinedOutput()

	return []archiveFile{f}
}

// gets current pod logs and logs from past containers
func archiveLogs(zw *zip.Writer, pods podList, baseDir string) error {

	// run kubectl logs and write to archive, accounts for containers in pod
	for _, pod := range pods.Items {
		fmt.Println("Archiving logs: ", pod.Metadata.Name, "Containers:", pod.Spec.Containers)
		for _, container := range pod.Spec.Containers {
			logs, err := zw.Create(baseDir + "/kubectl/pods/" + pod.Metadata.Name + "/" + container.Name + ".log")
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
				prev, err := zw.Create(baseDir + "/kubectl/pods/" + pod.Metadata.Name + "/" + "prev-" + container.Name + ".log")
				getPrevLogs.Stdout = prev
				if err != nil {
					return fmt.Errorf("failed to create podLogs.txt: %w", err)
				}
			}
		}
	}

	return nil
}

func archiveDescribes(zw *zip.Writer, pods podList, baseDir string) error {
	for _, pod := range pods.Items {
		describes, err := zw.Create(baseDir + "/kubectl/pods/" + pod.Metadata.Name + "/describe-" + pod.Metadata.Name + ".txt")
		if err != nil {
			return fmt.Errorf("failed to create podLogs.txt: %w", err)
		}

		describePod := exec.Command("kubectl", "describe", "pod", pod.Metadata.Name)
		describePod.Stdout = describes
		describePod.Stderr = os.Stderr

		if err := describePod.Run(); err != nil {
			return fmt.Errorf("failer to run describe pod: %w", err)
		}
	}
	return nil
}

func archiveManifests(zw *zip.Writer, pods podList, baseDir string) error {
	for _, pod := range pods.Items {
		manifests, err := zw.Create(baseDir + "/kubectl/pods/" + pod.Metadata.Name + "/manifest-" + pod.Metadata.Name + ".yaml")
		if err != nil {
			return fmt.Errorf("failed to create manifest.yaml: %w", err)
		}

		getManifest := exec.Command("kubectl", "get", "pod", pod.Metadata.Name, "-o", "yaml")
		getManifest.Stdout = manifests
		getManifest.Stderr = os.Stderr

		if err := getManifest.Run(); err != nil {
			fmt.Errorf("failed to get pod yaml: %w", err)
		}
	}
	return nil
}

/*
Docker functions


*/

func getContainers() (string, error) {

	containers, err := exec.Command("docker", "container", "ls", "--format", "{{.Names}}").Output()
	if err != nil {
		fmt.Errorf("failed to get container names with error: %w", err)
	}
	contStr := string(containers)
	fmt.Println(contStr)
	return contStr, err
}

/*
Graveyard
-----------
*/

//if err := archiveEvents(zw, baseDir); err != nil {
//	return fmt.Errorf("running archiveEvents failed: %w", err)
//}
//if err := archivePV(zw, baseDir); err != nil {
//	return fmt.Errorf("running archivePV failed: %w", err)
//}
//if err := archivePVC(zw, baseDir); err != nil {
//	return fmt.Errorf("running archivePV failed: %w", err)
//}
//if err := archiveLogs(zw, pods, baseDir); err != nil {
//	return fmt.Errorf("running archiveLogs failed: %w", err)
//}
//if err := archiveDescribes(zw, pods, baseDir); err != nil {
//	return fmt.Errorf("running archiveDescribes failed: %w", err)
//}
//if err := archiveManifests(zw, pods, baseDir); err != nil {
//	return fmt.Errorf("running archiveManifests failed: %w", err)
//}

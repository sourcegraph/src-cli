package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"unicode"

	"golang.org/x/sync/semaphore"

	"github.com/sourcegraph/sourcegraph/lib/errors"
	"github.com/sourcegraph/src-cli/internal/cmderrors"
	"github.com/sourcegraph/src-cli/internal/exec"
)

func init() {
	usage := `
'src debug kube' mocks kubectl commands to gather information about a kubernetes sourcegraph instance. 

Usage:

    src debug kube [command options]

Flags:

	-o			Specify the name of the output zip archive.
	-n			Specify the namespace passed to kubectl commands. If not specified the 'default' namespace is used.
	-cfg		Include Sourcegraph configuration json. Defaults to true.

Examples:

    $ src debug kube -o debug.zip

	$ src -v debug kube -n ns-sourcegraph -o foo

	$ src debug kube -cfg=false -o bar.zip

`

	flagSet := flag.NewFlagSet("kube", flag.ExitOnError)
	var base string
	var namespace string
	var configs bool
	flagSet.BoolVar(&configs, "cfg", true, "If true include Sourcegraph configuration files. Default value true.")
	flagSet.StringVar(&namespace, "n", "default", "The namespace passed to kubectl commands, if not specified the 'default' namespace is used")
	flagSet.StringVar(&base, "o", "debug.zip", "The name of the output zip archive")

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
		//TODO: improve formating to include 'ls' like pod listing for pods targeted.
		log.Printf("Archiving kubectl data for %d pods\n SRC_ENDPOINT: %v\n Context: %s Namespace: %v\n Output filename: %v", len(pods.Items), cfg.Endpoint, kubectx, namespace, base)

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

		err = archiveKube(ctx, zw, *verbose, configs, namespace, baseDir, pods)
		if err != nil {
			return cmderrors.ExitCode(1, err)
		}
		return nil
	}

	debugCommands = append(debugCommands, &command{
		flagSet: flagSet,
		handler: handler,
		usageFunc: func() {
			fmt.Println(usage)
		},
	})
}

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

// Run kubectl functions concurrently and archive results to zip file
func archiveKube(ctx context.Context, zw *zip.Writer, verbose, configs bool, namespace, baseDir string, pods podList) error {
	// Create a context with a cancel function that we call when returning
	// from archiveKube. This ensures we close all pending go-routines when returning
	// early because of an error.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// setup channel for slice of archive function outputs, as well as throttling semaphore
	ch := make(chan *archiveFile)
	wg := sync.WaitGroup{}
	semaphore := semaphore.NewWeighted(8)

	// create goroutine to get kubectl events
	wg.Add(1)
	go func() {
		if err := semaphore.Acquire(ctx, 1); err != nil {
			// return err
		}
		defer semaphore.Release(1)
		defer wg.Done()
		ch <- getEvents(ctx, namespace, baseDir)
	}()

	// create goroutine to get persistent volumes
	wg.Add(1)
	go func() {
		if err := semaphore.Acquire(ctx, 1); err != nil {
			// return err
		}
		defer semaphore.Release(1)
		defer wg.Done()
		ch <- getPV(ctx, namespace, baseDir)
	}()

	// create goroutine to get persistent volumes claim
	wg.Add(1)
	go func() {
		if err := semaphore.Acquire(ctx, 1); err != nil {
			// return err
		}
		defer semaphore.Release(1)
		defer wg.Done()
		ch <- getPVC(ctx, namespace, baseDir)
	}()

	// start goroutine to run kubectl logs for each pod's container's
	for _, pod := range pods.Items {
		for _, container := range pod.Spec.Containers {
			wg.Add(1)
			go func(pod, container string) {
				if err := semaphore.Acquire(ctx, 1); err != nil {
					// return err
				}
				defer semaphore.Release(1)
				defer wg.Done()
				ch <- getContainerLog(ctx, pod, container, namespace, baseDir)
			}(pod.Metadata.Name, container.Name)
		}
	}

	// start goroutine to run kubectl logs --previous for each pod's container's
	// won't write to zip on err, only passes bytes to channel if err not nil
	// TODO: It may be nice to store a list of pods for which --previous isn't collected, to be outputted with verbose flag
	for _, pod := range pods.Items {
		for _, container := range pod.Spec.Containers {
			wg.Add(1)
			go func(pod, container string) {
				if err := semaphore.Acquire(ctx, 1); err != nil {
					// return err
				}
				defer semaphore.Release(1)
				defer wg.Done()
				f := getPastContainerLog(ctx, pod, container, namespace, baseDir)
				if f.err == nil {
					ch <- f
				} else {
					if verbose {
						fmt.Printf("Could not gather --previous pod logs for: %s \nExited with err: %s\n", pod, f.err)
					}
				}
			}(pod.Metadata.Name, container.Name)
		}
	}

	// start goroutine for each pod to run kubectl describe pod
	for _, pod := range pods.Items {
		wg.Add(1)
		go func(pod string) {
			if err := semaphore.Acquire(ctx, 1); err != nil {
				// return err
			}
			defer semaphore.Release(1)
			defer wg.Done()
			ch <- getDescribe(ctx, pod, namespace, baseDir)
		}(pod.Metadata.Name)
	}

	// start goroutine for each pod to run kubectl get pod <pod> -o yaml
	for _, pod := range pods.Items {
		wg.Add(1)
		go func(pod string) {
			if err := semaphore.Acquire(ctx, 1); err != nil {
				// return err
			}
			defer semaphore.Release(1)
			defer wg.Done()
			ch <- getManifest(ctx, pod, namespace, baseDir)
		}(pod.Metadata.Name)
	}

	// start goroutine to get external service config
	if configs == true {
		wg.Add(1)
		go func() {
			if err := semaphore.Acquire(ctx, 1); err != nil {
				// return err
			}
			defer semaphore.Release(1)
			defer wg.Done()
			ch <- getExternalServicesConfig(ctx, baseDir)
		}()

		wg.Add(1)
		go func() {
			if err := semaphore.Acquire(ctx, 1); err != nil {
				// return err
			}
			defer semaphore.Release(1)
			defer wg.Done()
			ch <- getSiteConfig(ctx, baseDir)
		}()
	}

	// close channel when wait group goroutines have completed
	go func() {
		wg.Wait()
		close(ch)
	}()

	// write to archive all the outputs from kubectl call functions passed to buffer channel
	for f := range ch {
		if f.err != nil {
			log.Printf("getting data for %s failed: %v\noutput: %s", f.name, f.err, f.data)
			continue
		}

		if verbose {
			log.Printf("archiving file %q with %d bytes", f.name, len(f.data))
		}

		zf, err := zw.Create(f.name)
		if err != nil {
			return fmt.Errorf("failed to create %s: %w", f.name, err)
		}

		_, err = zf.Write(f.data)
		if err != nil {
			return fmt.Errorf("failed to write to %s: %w", f.name, err)
		}
	}

	return nil
}

func getPods(ctx context.Context, namespace string) (podList, error) {
	// Declare buffer type var for kubectl pipe
	var podsBuff bytes.Buffer

	// Get all pod names as json
	getPods := exec.CommandContext(ctx, "kubectl", "-n", namespace, "get", "pods", "-l", "deploy=sourcegraph", "-o=json")
	getPods.Stdout = &podsBuff
	getPods.Stderr = os.Stderr
	err := getPods.Run()

	//Declare struct to format decode from podList
	var pods podList

	//Decode json from podList
	if err := json.NewDecoder(&podsBuff).Decode(&pods); err != nil {
		fmt.Errorf("failed to unmarshall get pods json: %w", err)
	}

	return pods, err
}

func getEvents(ctx context.Context, namespace, baseDir string) *archiveFile {
	f := &archiveFile{name: baseDir + "/kubectl/events.txt"}
	f.data, f.err = exec.CommandContext(ctx, "kubectl", "-n", namespace, "get", "events").CombinedOutput()
	return f
}

func getPV(ctx context.Context, namespace, baseDir string) *archiveFile {
	f := &archiveFile{name: baseDir + "/kubectl/persistent-volumes.txt"}
	f.data, f.err = exec.CommandContext(ctx, "kubectl", "-n", namespace, "get", "pv").CombinedOutput()
	return f
}

func getPVC(ctx context.Context, namespace, baseDir string) *archiveFile {
	f := &archiveFile{name: baseDir + "/kubectl/persistent-volume-claims.txt"}
	f.data, f.err = exec.CommandContext(ctx, "kubectl", "-n", namespace, "get", "pvc").CombinedOutput()
	return f
}

// get kubectl logs for pod containers
func getContainerLog(ctx context.Context, podName, containerName, namespace, baseDir string) *archiveFile {
	f := &archiveFile{name: baseDir + "/kubectl/pods/" + podName + "/" + containerName + ".log"}
	f.data, f.err = exec.CommandContext(ctx, "kubectl", "-n", namespace, "logs", podName, "-c", containerName).CombinedOutput()
	return f
}

// get kubectl logs for past container
func getPastContainerLog(ctx context.Context, podName, containerName, namespace, baseDir string) *archiveFile {
	f := &archiveFile{name: baseDir + "/kubectl/pods/" + podName + "/" + "prev-" + containerName + ".log"}
	cmdStr := []string{"kubectl", "-n", namespace, "logs", "--previous", podName, "-c", containerName}
	f.data, f.err = exec.CommandContext(ctx, "kubectl", "-n", namespace, "logs", "--previous", podName, "-c", containerName).CombinedOutput()
	if f.err != nil {
		f.err = errors.Wrapf(f.err, "executing command: %s: received error: %s", cmdStr, f.data)
	}
	return f
}

func getDescribe(ctx context.Context, podName, namespace, baseDir string) *archiveFile {
	f := &archiveFile{name: baseDir + "/kubectl/pods/" + podName + "/describe-" + podName + ".txt"}
	f.data, f.err = exec.CommandContext(ctx, "kubectl", "-n", namespace, "describe", "pod", podName).CombinedOutput()
	return f
}

func getManifest(ctx context.Context, podName, namespace, baseDir string) *archiveFile {
	f := &archiveFile{name: baseDir + "/kubectl/pods/" + podName + "/manifest-" + podName + ".yaml"}
	f.data, f.err = exec.CommandContext(ctx, "kubectl", "-n", namespace, "get", "pod", podName, "-o", "yaml").CombinedOutput()
	return f
}

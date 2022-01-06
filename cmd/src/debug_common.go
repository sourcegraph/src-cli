package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"syscall"

	"github.com/sourcegraph/src-cli/internal/exec"
)

type archiveFile struct {
	name string
	data []byte
	err  error
}

// setOpenFileLimits increases the limit of open files to the given number. This is needed
// when doings lots of concurrent network requests which establish open sockets.
func setOpenFileLimits(n uint64) error {
	var rlimit syscall.Rlimit
	err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rlimit)
	if err != nil {
		return err
	}

	rlimit.Max = n
	rlimit.Cur = n

	return syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rlimit)
}

// setupDebug takes the name of a base directory and returns the file pipe, zip writer,
// and context needed for later archive functions. Don't forget to defer close on these
// after calling setupDebug!
func setupDebug(base string) (*os.File, *zip.Writer, context.Context, error) {
	// open pipe to output file
	out, err := os.OpenFile(base, os.O_CREATE|os.O_RDWR|os.O_EXCL, 0666)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to open file: %w", err)
	}
	// increase limit of open files
	err = setOpenFileLimits(64000)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to set open file limits: %w", err)
	}
	// init zip writer
	zw := zip.NewWriter(out)
	// init context
	ctx := context.Background()

	return out, zw, ctx, err
}

/*
Kubernetes stuff
TODO: handle namespaces, remove --all-namespaces from get events
*/

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
func archiveKube(ctx context.Context, zw *zip.Writer, verbose bool, baseDir string) error {
	// Create a context with a cancel function that we call when returning
	// from archiveKube. This ensures we close all pending go-routines when returning
	// early because of an error.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	pods, err := getPods(ctx)
	if err != nil {
		return fmt.Errorf("failed to get pods: %w", err)
	}

	if verbose {
		log.Printf("getting kubectl data for %d pods...\n", len(pods.Items))
	}

	// setup channel for slice of archive function outputs
	ch := make(chan *archiveFile)
	wg := sync.WaitGroup{}

	// create goroutine to get kubectl events
	wg.Add(1)
	go func() {
		defer wg.Done()
		ch <- getEvents(ctx, baseDir)
	}()

	// create goroutine to get persistent volumes
	wg.Add(1)
	go func() {
		defer wg.Done()
		ch <- getPV(ctx, baseDir)
	}()

	// create goroutine to get persistent volumes claim
	wg.Add(1)
	go func() {
		defer wg.Done()
		ch <- getPVC(ctx, baseDir)
	}()

	// start goroutine to run kubectl logs for each pod's container's
	for _, pod := range pods.Items {
		for _, container := range pod.Spec.Containers {
			wg.Add(1)
			go func(pod, container string) {
				defer wg.Done()
				ch <- getContainerLog(ctx, pod, container, baseDir)
			}(pod.Metadata.Name, container.Name)
		}
	}

	// start goroutine to run kubectl logs --previous for each pod's container's
	// won't write to zip on err, only passes bytes to channel if err not nil
	for _, pod := range pods.Items {
		for _, container := range pod.Spec.Containers {
			wg.Add(1)
			go func(pod, container string) {
				defer wg.Done()
				f := getPastContainerLog(ctx, pod, container, baseDir)
				if f.err == nil {
					ch <- f
				}
			}(pod.Metadata.Name, container.Name)
		}
	}

	// start goroutine for each pod to run kubectl describe pod
	for _, pod := range pods.Items {
		wg.Add(1)
		go func(pod string) {
			defer wg.Done()
			ch <- getDescribe(ctx, pod, baseDir)
		}(pod.Metadata.Name)
	}

	// start goroutine for each pod to run kubectl get pod <pod> -o yaml
	for _, pod := range pods.Items {
		wg.Add(1)
		go func(pod string) {
			defer wg.Done()
			ch <- getManifest(ctx, pod, baseDir)
		}(pod.Metadata.Name)
	}

	// close channel when wait group goroutines have completed
	go func() {
		wg.Wait()
		close(ch)
	}()

	// write to archive all the outputs from kubectl call functions passed to buffer channel
	for f := range ch {
		if f.err != nil {
			return fmt.Errorf("aborting due to error on %s: %v\noutput: %s", f.name, f.err, f.data)
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

func getPods(ctx context.Context) (podList, error) {
	// Declare buffer type var for kubectl pipe
	var podsBuff bytes.Buffer

	// Get all pod names as json
	getPods := exec.CommandContext(ctx, "kubectl", "get", "pods", "-l", "deploy=sourcegraph", "-o=json")
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

func getEvents(ctx context.Context, baseDir string) *archiveFile {
	f := &archiveFile{name: baseDir + "/kubectl/events.txt"}
	f.data, f.err = exec.CommandContext(ctx, "kubectl", "get", "events", "--all-namespaces").CombinedOutput()
	return f
}

func getPV(ctx context.Context, baseDir string) *archiveFile {
	f := &archiveFile{name: baseDir + "/kubectl/persistent-volumes.txt"}
	f.data, f.err = exec.CommandContext(ctx, "kubectl", "get", "pv").CombinedOutput()
	return f
}

func getPVC(ctx context.Context, baseDir string) *archiveFile {
	f := &archiveFile{name: baseDir + "/kubectl/persistent-volume-claims.txt"}
	f.data, f.err = exec.CommandContext(ctx, "kubectl", "get", "pvc").CombinedOutput()
	return f
}

// get kubectl logs for pod containers
func getContainerLog(ctx context.Context, podName, containerName, baseDir string) *archiveFile {
	f := &archiveFile{name: baseDir + "/kubectl/pods/" + podName + "/" + containerName + ".log"}
	f.data, f.err = exec.CommandContext(ctx, "kubectl", "logs", podName, "-c", containerName).CombinedOutput()
	return f
}

// get kubectl logs for past container
func getPastContainerLog(ctx context.Context, podName, containerName, baseDir string) *archiveFile {
	f := &archiveFile{name: baseDir + "/kubectl/pods/" + podName + "/" + "prev-" + containerName + ".log"}
	f.data, f.err = exec.CommandContext(ctx, "kubectl", "logs", "--previous", podName, "-c", containerName).CombinedOutput()
	return f
}

func getDescribe(ctx context.Context, podName, baseDir string) *archiveFile {
	f := &archiveFile{name: baseDir + "/kubectl/pods/" + podName + "/describe-" + podName + ".txt"}
	f.data, f.err = exec.CommandContext(ctx, "kubectl", "describe", "pod", podName).CombinedOutput()
	return f
}

func getManifest(ctx context.Context, podName, baseDir string) *archiveFile {
	f := &archiveFile{name: baseDir + "/kubectl/pods/" + podName + "/manifest-" + podName + ".yaml"}
	f.data, f.err = exec.CommandContext(ctx, "kubectl", "get", "pod", podName, "-o", "yaml").CombinedOutput()
	return f
}

/*
Docker functions
TODO: handle for single container instance
*/

func archiveDocker(ctx context.Context, zw *zip.Writer, verbose bool, baseDir string) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	containers, err := getContainers(ctx)
	if err != nil {
		return fmt.Errorf("failed to get docker containers: %w", err)
	}

	if verbose {
		log.Printf("getting docker data for %d containers...\n", len(containers))
	}

	// setup channel for slice of archive function outputs
	ch := make(chan *archiveFile)
	wg := sync.WaitGroup{}

	// start goroutine to run docker container stats --no-stream
	wg.Add(1)
	go func() {
		defer wg.Done()
		ch <- getStats(ctx, baseDir)
	}()

	// start goroutine to run docker container logs <container>
	for _, container := range containers {
		wg.Add(1)
		go func(container string) {
			defer wg.Done()
			ch <- getLog(ctx, container, baseDir)
		}(container)
	}

	// start goroutine to run docker container inspect <container>
	for _, container := range containers {
		wg.Add(1)
		go func(container string) {
			defer wg.Done()
			ch <- getInspect(ctx, container, baseDir)
		}(container)
	}

	// close channel when wait group goroutines have completed
	go func() {
		wg.Wait()
		close(ch)
	}()

	for f := range ch {
		if f.err != nil {
			return fmt.Errorf("aborting due to error on %s: %v\noutput: %s", f.name, f.err, f.data)
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

func getContainers(ctx context.Context) ([]string, error) {
	c, err := exec.CommandContext(ctx, "docker", "container", "ls", "--format", "{{.Names}}").Output()
	if err != nil {
		fmt.Errorf("failed to get container names with error: %w", err)
	}
	s := string(c)
	containers := strings.Split(strings.TrimSpace(s), "\n")
	fmt.Println(containers)
	return containers, err
}

func getLog(ctx context.Context, container, baseDir string) *archiveFile {
	f := &archiveFile{name: baseDir + "/docker/containers/" + container + "/" + container + ".log"}
	f.data, f.err = exec.CommandContext(ctx, "docker", "container", "logs", container).CombinedOutput()
	return f
}

func getInspect(ctx context.Context, container, baseDir string) *archiveFile {
	f := &archiveFile{name: baseDir + "/docker/containers/" + container + "/inspect-" + container + ".txt"}
	f.data, f.err = exec.CommandContext(ctx, "docker", "container", "inspect", container).CombinedOutput()
	return f
}

func getStats(ctx context.Context, baseDir string) *archiveFile {
	f := &archiveFile{name: baseDir + "/docker/stats.txt"}
	f.data, f.err = exec.CommandContext(ctx, "docker", "container", "stats", "--no-stream").CombinedOutput()
	return f
}

// TODO api brainstorm
// Perform the request.
/* var result interface{}
if ok, err := cfg.apiClient(apiFlags, flagSet.Output()).NewRequest(query, vars).DoRaw(context.Background(), &result); err != nil || !ok {
return err
} */

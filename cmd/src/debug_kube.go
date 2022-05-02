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
	"path/filepath"
	"strings"

	"golang.org/x/sync/errgroup"

	"golang.org/x/sync/semaphore"

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
		if !strings.HasSuffix(base, ".zip") {
			baseDir = base
			base = base + ".zip"
		} else {
			baseDir = strings.TrimSuffix(base, ".zip")
		}

		// init context
		ctx := context.Background()
		// open pipe to output file
		out, err := os.OpenFile(base, os.O_CREATE|os.O_RDWR|os.O_EXCL, 0666)
		if err != nil {
			fmt.Errorf("failed to open file: %w", err)
		}
		defer out.Close()
		// init zip writer
		zw := zip.NewWriter(out)
		defer zw.Close()

		//TODO: improve formating to include 'ls' like pod listing for pods targeted.

		// Gather data for safety check
		pods, err := selectPods(ctx, namespace)
		if err != nil {
			return fmt.Errorf("failed to get pods: %w", err)
		}
		kubectx, err := exec.CommandContext(ctx, "kubectl", "config", "current-context").CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to get current-context: %w", err)
		}
		// Safety check user knows what they've targeted with this command
		log.Printf("Archiving kubectl data for %d pods\n SRC_ENDPOINT: %v\n Context: %s Namespace: %v\n Output filename: %v", len(pods.Items), cfg.Endpoint, kubectx, namespace, base)
		if verified, _ := verify("Do you want to start writing to an archive?"); !verified {
			return nil
		}

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
	g, ctx := errgroup.WithContext(ctx)
	semaphore := semaphore.NewWeighted(8)

	// create goroutine to get pods
	g.Go(func() error {
		if err := semaphore.Acquire(ctx, 1); err != nil {
			return err
		}
		defer semaphore.Release(1)
		ch <- getPods(ctx, namespace, baseDir)
		return nil
	})

	// create goroutine to get kubectl events
	g.Go(func() error {
		if err := semaphore.Acquire(ctx, 1); err != nil {
			return err
		}
		defer semaphore.Release(1)
		ch <- getEvents(ctx, namespace, baseDir)
		return nil
	})

	// create goroutine to get persistent volumes
	g.Go(func() error {
		if err := semaphore.Acquire(ctx, 1); err != nil {
			return err
		}
		defer semaphore.Release(1)
		ch <- getPV(ctx, namespace, baseDir)
		return nil
	})

	// create goroutine to get persistent volumes claim
	g.Go(func() error {
		if err := semaphore.Acquire(ctx, 1); err != nil {
			return err
		}
		defer semaphore.Release(1)
		ch <- getPVC(ctx, namespace, baseDir)
		return nil
	})

	// start goroutine to run kubectl logs for each pod's container's
	for _, pod := range pods.Items {
		for _, container := range pod.Spec.Containers {
			p := pod.Metadata.Name
			c := container.Name
			g.Go(func() error {
				if err := semaphore.Acquire(ctx, 1); err != nil {
					return err
				}
				defer semaphore.Release(1)
				ch <- getPodLog(ctx, p, c, namespace, baseDir)
				return nil
			})
		}
	}

	// start goroutine to run kubectl logs --previous for each pod's container's
	// won't write to zip on err, only passes bytes to channel if err not nil
	// TODO: It may be nice to store a list of pods for which --previous isn't collected, to be outputted with verbose flag
	for _, pod := range pods.Items {
		for _, container := range pod.Spec.Containers {
			p := pod.Metadata.Name
			c := container.Name
			g.Go(func() error {
				if err := semaphore.Acquire(ctx, 1); err != nil {
					return err
				}
				defer semaphore.Release(1)
				f := getPastPodLog(ctx, p, c, namespace, baseDir)
				if f.err == nil {
					ch <- f
				} else if verbose {
					fmt.Printf("Could not gather --previous pod logs for: %s \nExited with err: %s\n", p, f.err)
				}
				return nil
			})
		}
	}

	// start goroutine for each pod to run kubectl describe pod
	for _, pod := range pods.Items {
		p := pod.Metadata.Name
		g.Go(func() error {
			if err := semaphore.Acquire(ctx, 1); err != nil {
				return err
			}
			defer semaphore.Release(1)
			ch <- getDescribe(ctx, p, namespace, baseDir)
			return nil
		})
	}

	// start goroutine for each pod to run kubectl get pod <pod> -o yaml
	for _, pod := range pods.Items {
		p := pod.Metadata.Name
		g.Go(func() error {
			if err := semaphore.Acquire(ctx, 1); err != nil {
				return err
			}
			defer semaphore.Release(1)
			ch <- getManifest(ctx, p, namespace, baseDir)
			return nil
		})
	}

	// start goroutine to get external service config
	if configs {
		g.Go(func() error {
			if err := semaphore.Acquire(ctx, 1); err != nil {
				return err
			}
			defer semaphore.Release(1)
			ch <- getExternalServicesConfig(ctx, baseDir)
			return nil
		})

		g.Go(func() error {
			if err := semaphore.Acquire(ctx, 1); err != nil {
				return err
			}
			defer semaphore.Release(1)
			ch <- getSiteConfig(ctx, baseDir)
			return nil
		})
	}

	// close channel when wait group goroutines have completed
	go func() {
		g.Wait()
		close(ch)
	}()

	// Read binaries from channel and write to archive on host machine
	if err := writeChannelContentsToZip(zw, ch, verbose); err != nil {
		return fmt.Errorf("failed to write archives from channel: %w", err)
	}

	return nil
}

func selectPods(ctx context.Context, namespace string) (podList, error) {
	// Declare buffer type var for kubectl pipe
	var podsBuff bytes.Buffer

	// Get all pod names as json
	podsCmd := exec.CommandContext(
		ctx,
		"kubectl", "-n", namespace, "get", "pods", "-l", "deploy=sourcegraph", "-o=json",
	)
	podsCmd.Stdout = &podsBuff
	podsCmd.Stderr = os.Stderr
	err := podsCmd.Run()
	if err != nil {
		fmt.Errorf("failed to aquire pods for subcommands with err: %v", err)
	}

	//Decode json from podList
	var pods podList
	if err := json.NewDecoder(&podsBuff).Decode(&pods); err != nil {
		fmt.Errorf("failed to unmarshall get pods json: %w", err)
	}

	return pods, err
}

func getPods(ctx context.Context, namespace, baseDir string) *archiveFile {
	return archiveFileFromCommand(
		ctx,
		filepath.Join(baseDir, "kubectl", "getPods.txt"),
		"kubectl", "-n", namespace, "get", "pods", "-o", "wide",
	)
}

func getEvents(ctx context.Context, namespace, baseDir string) *archiveFile {
	return archiveFileFromCommand(
		ctx,
		filepath.Join(baseDir, "kubectl", "events.txt"),
		"kubectl", "-n", namespace, "get", "events",
	)
}

func getPV(ctx context.Context, namespace, baseDir string) *archiveFile {
	return archiveFileFromCommand(
		ctx,
		filepath.Join(baseDir, "kubectl", "persistent-volumes.txt"),
		"kubectl", "-n", namespace, "get", "pv",
	)
}

func getPVC(ctx context.Context, namespace, baseDir string) *archiveFile {
	return archiveFileFromCommand(
		ctx,
		filepath.Join(baseDir, "kubectl", "persistent-volume-claims.txt"),
		"kubectl", "-n", namespace, "get", "pvc",
	)
}

// get kubectl logs for pod containers
func getPodLog(ctx context.Context, podName, containerName, namespace, baseDir string) *archiveFile {
	return archiveFileFromCommand(
		ctx,
		filepath.Join(baseDir, "kubectl", "pods", podName, fmt.Sprintf("%v.log", containerName)),
		"kubectl", "-n", namespace, "logs", podName, "-c", containerName,
	)
}

// get kubectl logs for past container
func getPastPodLog(ctx context.Context, podName, containerName, namespace, baseDir string) *archiveFile {
	return archiveFileFromCommand(
		ctx,
		filepath.Join(baseDir, "kubectl", "pods", podName, fmt.Sprintf("prev-%v.log", containerName)),
		"kubectl", "-n", namespace, "logs", "--previous", podName, "-c", containerName,
	)
}

func getDescribe(ctx context.Context, podName, namespace, baseDir string) *archiveFile {
	return archiveFileFromCommand(
		ctx,
		filepath.Join(baseDir, "kubectl", "pods", podName, fmt.Sprintf("describe-%v.txt", podName)),
		"kubectl", "-n", namespace, "describe", "pod", podName,
	)
}

func getManifest(ctx context.Context, podName, namespace, baseDir string) *archiveFile {
	return archiveFileFromCommand(
		ctx,
		filepath.Join(baseDir, "kubectl", "pods", podName, fmt.Sprintf("manifest-%v.yaml", podName)),
		"kubectl", "-n", namespace, "get", "pod", podName, "-o", "yaml",
	)
}

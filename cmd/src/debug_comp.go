package main

import (
	"archive/zip"
	"context"
	"flag"
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/sync/errgroup"

	"golang.org/x/sync/semaphore"

	"github.com/sourcegraph/src-cli/internal/cmderrors"
)

func init() {
	usage := `
'src debug comp' mocks docker cli commands to gather information about a docker-compose Sourcegraph instance.

Usage:

    src debug comp [command options]

Flags:

	-o			Specify the name of the output zip archive.
	-cfg		Include Sourcegraph configuration json. Defaults to true.

Examples:

    $ src debug comp -o debug.zip

	$ src -v debug comp -cfg=false -o foo.zip

`

	flagSet := flag.NewFlagSet("comp", flag.ExitOnError)
	var base string
	var configs bool
	flagSet.BoolVar(&configs, "cfg", true, "If true include Sourcegraph configuration files. Default value true.")
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

		ctx := context.Background()

		//Gather data for safety check
		containers, err := getContainers(ctx)
		if err != nil {
			fmt.Errorf("failed to get containers for subcommand with err: %v", err)
		}
		// Safety check user knows what they are targeting with this debug command
		log.Printf("This command will archive docker-cli data for %d containers\n SRC_ENDPOINT: %v\n Output filename: %v", len(containers), cfg.Endpoint, base)
		if verified, _ := verify("Do you want to start writing to an archive?"); !verified {
			return nil
		}

		out, zw, ctx, err := setupDebug(base)
		if err != nil {
			return fmt.Errorf("failed to open file: %w", err)
		}
		defer out.Close()
		defer zw.Close()

		err = archiveDocker(ctx, zw, *verbose, configs, baseDir)
		if err != nil {
			return cmderrors.ExitCode(1, nil)
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

/*
Docker functions
*/

func archiveDocker(ctx context.Context, zw *zip.Writer, verbose, configs bool, baseDir string) error {
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
	g, ctx := errgroup.WithContext(ctx)
	semaphore := semaphore.NewWeighted(8)

	// start goroutine to run docker ps -o wide
	g.Go(func() error {
		if err := semaphore.Acquire(ctx, 1); err != nil {
			return err
		}
		defer semaphore.Release(1)
		ch <- getPs(ctx, baseDir)
		return nil
	})

	// start goroutine to run docker container stats --no-stream
	g.Go(func() error {
		if err := semaphore.Acquire(ctx, 1); err != nil {
			return err
		}
		defer semaphore.Release(1)
		ch <- getStats(ctx, baseDir)
		return nil
	})

	// start goroutine to run docker container logs <container>
	for _, container := range containers {
		c := container
		g.Go(func() error {
			if err := semaphore.Acquire(ctx, 1); err != nil {
				return err
			}
			defer semaphore.Release(1)
			ch <- getContainerLog(ctx, c, baseDir)
			return nil
		})
	}

	// start goroutine to run docker container inspect <container>
	for _, container := range containers {
		c := container
		g.Go(func() error {
			if err := semaphore.Acquire(ctx, 1); err != nil {
				return err
			}
			defer semaphore.Release(1)
			ch <- getInspect(ctx, c, baseDir)
			return nil
		})
	}

	// start goroutine to get configs
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

func getContainers(ctx context.Context) ([]string, error) {
	c, err := exec.CommandContext(ctx, "docker", "container", "ls", "--format", "{{.Names}} {{.Networks}}").Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get container names with error: %w", err)
	}
	s := string(c)
	preprocessed := strings.Split(strings.TrimSpace(s), "\n")
	containers := []string{}
	for _, container := range preprocessed {
		tmpStr := strings.Split(container, " ")
		if tmpStr[1] == "docker-compose_sourcegraph" {
			containers = append(containers, tmpStr[0])
		}
	}
	return containers, err
}

func getPs(ctx context.Context, baseDir string) *archiveFile {
	return archiveFileFromCommand(
		ctx,
		filepath.Join(baseDir, "docker", "docker-ps.txt"),
		"docker", "ps", "--filter", "network=docker-compose_sourcegraph",
	)
}

func getContainerLog(ctx context.Context, container, baseDir string) *archiveFile {
	return archiveFileFromCommand(
		ctx,
		filepath.Join(baseDir, "docker", "containers", container, fmt.Sprintf("%v.log", container)),
		"docker", "container", "logs", container,
	)
}

func getInspect(ctx context.Context, container, baseDir string) *archiveFile {
	return archiveFileFromCommand(
		ctx,
		filepath.Join(baseDir, "docker", "containers", container, fmt.Sprintf("inspect-%v.txt", container)),
		"docker", "container", "inspect", container,
	)
}

func getStats(ctx context.Context, baseDir string) *archiveFile {
	return archiveFileFromCommand(
		ctx,
		filepath.Join(baseDir, "docker", "stats.txt"),
		"docker", "container", "stats", "--no-stream",
	)
}

package main

import (
	"archive/zip"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sourcegraph/sourcegraph/lib/errors"

	"golang.org/x/sync/errgroup"

	"golang.org/x/sync/semaphore"

	"github.com/sourcegraph/src-cli/internal/cmderrors"
)

func init() {
	usage := `
'src debug compose' invokes docker cli diagnostic commands targeting a containers that are members of a docker-compose network, 
writing an archive file from their returns. 

Usage:

	src debug compose [command options]

Flags:

	-o					Specify the name of the output zip archive.
	--no-configs		Don't include Sourcegraph configuration json.

Examples:

	$ src debug compose -o debug.zip

	$ src -v debug compose -no-configs -o foo.zip

`

	flagSet := flag.NewFlagSet("compose", flag.ExitOnError)
	var base string
	var noConfigs bool
	flagSet.StringVar(&base, "o", "debug.zip", "The name of the output zip archive")
	flagSet.BoolVar(&noConfigs, "no-configs", false, "If true include Sourcegraph configuration files. Default value true.")

	handler := func(args []string) error {
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		// process -o flag to get zipfile and base directory names
		if base == "" {
			return fmt.Errorf("empty -o flag")
		}
		// declare basedir for archive file structure
		base, baseDir := processBaseDir(base)

		// init context
		ctx := context.Background()
		// open pipe to output file
		out, err := os.OpenFile(base, os.O_CREATE|os.O_RDWR|os.O_EXCL, 0666)
		if err != nil {
			return errors.Wrapf(err, "failed to open file: %w", err)
		}
		defer out.Close()
		// init zip writer
		zw := zip.NewWriter(out)
		defer zw.Close()

		//Gather data for safety check
		containers, err := getContainers(ctx)
		if err != nil {
			return errors.Wrapf(err, "failed to get containers for subcommand with err: %v", err)
		}
		// Safety check user knows what they are targeting with this debug command
		log.Printf("This command will archive docker-cli data for %d containers\n SRC_ENDPOINT: %v\n Output filename: %v", len(containers), cfg.Endpoint, base)
		if verified, _ := verify("Do you want to start writing to an archive?"); !verified {
			return nil
		}

		err = archiveCompose(ctx, zw, *verbose, noConfigs, baseDir)
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

// writes archive of common docker cli commands
func archiveCompose(ctx context.Context, zw *zip.Writer, verbose, noConfigs bool, baseDir string) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	containers, err := getContainers(ctx)
	if err != nil {
		return errors.Wrapf(err, "failed to get docker containers: %w", err)
	}

	if verbose {
		log.Printf("getting docker data for %d containers...\n", len(containers))
	}

	// setup channel for slice of archive function outputs
	ch := make(chan *archiveFile)
	g, ctx := errgroup.WithContext(ctx)
	semaphore := semaphore.NewWeighted(8)

	run := func(f func() error) {
		g.Go(func() error {
			if err := semaphore.Acquire(ctx, 1); err != nil {
				return err
			}
			defer semaphore.Release(1)

			return f()
		})
	}

	// start goroutine to run docker ps -o wide
	run(func() error {
		ch <- getPs(ctx, baseDir)
		return nil
	})

	// start goroutine to run docker container stats --no-stream
	run(func() error {
		ch <- getStats(ctx, baseDir)
		return nil
	})

	// start goroutine to run docker container logs <container>
	for _, container := range containers {
		container := container
		run(func() error {
			ch <- getContainerLog(ctx, container, baseDir)
			return nil
		})
	}

	// start goroutine to run docker container inspect <container>
	for _, container := range containers {
		container := container
		run(func() error {
			ch <- getInspect(ctx, container, baseDir)
			return nil
		})
	}

	// start goroutine to get configs
	if !noConfigs {
		run(func() error {
			ch <- getExternalServicesConfig(ctx, baseDir)
			return nil
		})

		run(func() error {
			ch <- getSiteConfig(ctx, baseDir)
			return nil
		})
	}

	// close channel when wait group goroutines have completed
	go func() {
		if err := g.Wait(); err != nil {
			fmt.Printf("archiveCompose failed to open wait group: %v", err)
		}
		close(ch)
	}()

	// Read binaries from channel and write to archive on host machine
	if err := writeChannelContentsToZip(zw, ch, verbose); err != nil {
		return errors.Wrap(err, "failed to write archives from channel")
	}

	return nil
}

// Returns list of containers that are members of the docker-compose_sourcegraph
func getContainers(ctx context.Context) ([]string, error) {
	c, err := exec.CommandContext(ctx, "docker", "container", "ls", "--format", "{{.Names}} {{.Networks}}").Output()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get container names with error: %w", err)
	}
	s := string(c)
	preprocessed := strings.Split(strings.TrimSpace(s), "\n")
	containers := make([]string, 0, len(preprocessed))
	for _, container := range preprocessed {
		tmpStr := strings.Split(container, " ")
		if tmpStr[1] == "docker-compose_sourcegraph" {
			containers = append(containers, tmpStr[0])
		}
	}
	return containers, err
}

// runs archiveFileFromCommand with args docker ps
func getPs(ctx context.Context, baseDir string) *archiveFile {
	return archiveFileFromCommand(
		ctx,
		filepath.Join(baseDir, "docker", "docker-ps.txt"),
		"docker", "ps", "--filter", "network=docker-compose_sourcegraph",
	)
}

// runs archiveFileFromCommand with args docker container stats
func getStats(ctx context.Context, baseDir string) *archiveFile {
	return archiveFileFromCommand(
		ctx,
		filepath.Join(baseDir, "docker", "stats.txt"),
		"docker", "container", "stats", "--no-stream",
	)
}

// runs archiveFileFromCommand with args docker container logs $CONTAINER
func getContainerLog(ctx context.Context, container, baseDir string) *archiveFile {
	return archiveFileFromCommand(
		ctx,
		filepath.Join(baseDir, "docker", "containers", container, fmt.Sprintf("%s.log", container)),
		"docker", "container", "logs", container,
	)
}

// runs archiveFileFromCommand with args docker container inspect $CONTAINER
func getInspect(ctx context.Context, container, baseDir string) *archiveFile {
	return archiveFileFromCommand(
		ctx,
		filepath.Join(baseDir, "docker", "containers", container, fmt.Sprintf("inspect-%s.txt", container)),
		"docker", "container", "inspect", container,
	)
}

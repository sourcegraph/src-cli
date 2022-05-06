package main

import (
	"archive/zip"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"

	"github.com/sourcegraph/sourcegraph/lib/errors"
	"github.com/sourcegraph/src-cli/internal/cmderrors"
)

func init() {
	usage := `
'src debug server' invokes docker cli diagnostic commands targeting a Sourcegraph server container, 
and writes an archive file from their returns. 

Usage:

    src debug serv [command options]

Flags:

	-o			    Specify the name of the output zip archive.
	-no-config		Don't include Sourcegraph configuration json.

Examples:

    $ src debug server -c foo -o debug.zip

	$ src -v debug server --no-configs -c ViktorVaughn -o foo.zip

`

	flagSet := flag.NewFlagSet("server", flag.ExitOnError)
	var base string
	var container string
	var noConfigs bool
	flagSet.StringVar(&base, "o", "debug.zip", "The name of the output zip archive")
	flagSet.StringVar(&container, "c", "", "The container to target")
	flagSet.BoolVar(&noConfigs, "no-configs", false, "If true include Sourcegraph configuration files. Default value true.")

	handler := func(args []string) error {
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		//process -o flag to get zipfile and base directory names, make sure container is targeted
		if base == "" {
			return fmt.Errorf("empty -o flag")
		}
		if container == "" {
			return fmt.Errorf("empty -c flag, specify a container: src debug server -c foo")
		}
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

		// Safety check user knows what they are targeting with this debug command
		log.Printf("This command will archive docker-cli data for container: %s\n SRC_ENDPOINT: %s\n Output filename: %s", container, cfg.Endpoint, base)
		if verified, _ := verify("Do you want to start writing to an archive?"); !verified {
			return nil
		}

		err = archiveServ(ctx, zw, *verbose, noConfigs, container, baseDir)
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

// Runs common docker cli commands on a single container
func archiveServ(ctx context.Context, zw *zip.Writer, verbose, noConfigs bool, container, baseDir string) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

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
		ch <- getServLog(ctx, container, baseDir)
		return nil
	})

	// start goroutine to run docker ps -o wide
	run(func() error {
		ch <- getServInspect(ctx, container, baseDir)
		return nil
	})

	// start goroutine to run docker ps -o wide
	run(func() error {
		ch <- getServTop(ctx, container, baseDir)
		return nil
	})

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
		g.Wait()
		close(ch)
	}()

	// Read binaries from channel and write to archive on host machine
	if err := writeChannelContentsToZip(zw, ch, verbose); err != nil {
		return errors.Wrapf(err, "failed to write archives from channel: %w", err)
	}

	return nil
}

// runs archiveFileFromCommand with args container logs $CONTAINER
func getServLog(ctx context.Context, container, baseDir string) *archiveFile {
	return archiveFileFromCommand(
		ctx,
		filepath.Join(baseDir, fmt.Sprintf("%s.log", container)),
		"docker", "container", "logs", container,
	)
}

// runs archiveFileFromCommand with args container inspect $CONTAINER
func getServInspect(ctx context.Context, container, baseDir string) *archiveFile {
	return archiveFileFromCommand(
		ctx,
		filepath.Join(baseDir, fmt.Sprintf("inspect-%s.txt", container)),
		"docker", "container", "inspect", container,
	)
}

// runs archiveFileFromCommand with args top $CONTAINER
func getServTop(ctx context.Context, container, baseDir string) *archiveFile {
	return archiveFileFromCommand(
		ctx,
		filepath.Join(baseDir, fmt.Sprintf("top-%s.txt", container)),
		"docker", "top", container,
	)
}

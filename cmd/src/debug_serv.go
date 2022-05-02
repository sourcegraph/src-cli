package main

import (
	"archive/zip"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"

	"github.com/sourcegraph/src-cli/internal/cmderrors"
)

func init() {
	usage := `
'src debug serv' mocks docker cli commands to gather information about a Sourcegraph server instance. 

Usage:

    src debug serv [command options]

Flags:

	-o			Specify the name of the output zip archive.
	-cfg		Include Sourcegraph configuration json. Defaults to true.

Examples:

    $ src debug serv -c foo -o debug.zip

	$ src -v debug serv -cfg=false -c ViktorVaughn -o foo.zip

`

	flagSet := flag.NewFlagSet("serv", flag.ExitOnError)
	var base string
	var container string
	var configs bool
	flagSet.BoolVar(&configs, "cfg", true, "If true include Sourcegraph configuration files. Default value true.")
	flagSet.StringVar(&base, "o", "debug.zip", "The name of the output zip archive")
	flagSet.StringVar(&container, "c", "", "The container to target")

	handler := func(args []string) error {
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		//validate required flags aren't empty
		if base == "" {
			return fmt.Errorf("empty -o flag")
		}
		if container == "" {
			return fmt.Errorf("empty -c flag, specifying a container is required")
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

		// Safety check user knows what they are targeting with this debug command
		log.Printf("This command will archive docker-cli data for container: %v\n SRC_ENDPOINT: %v\n Output filename: %v", container, cfg.Endpoint, base)
		if verified, _ := verify("Do you want to start writing to an archive?"); !verified {
			return nil
		}

		err = archiveServ(ctx, zw, *verbose, configs, container, baseDir)
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

func archiveServ(ctx context.Context, zw *zip.Writer, verbose, configs bool, container, baseDir string) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

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
		ch <- getServLog(ctx, container, baseDir)
		return nil
	})

	// start goroutine to run docker ps -o wide
	g.Go(func() error {
		if err := semaphore.Acquire(ctx, 1); err != nil {
			return err
		}
		defer semaphore.Release(1)
		ch <- getServInspect(ctx, container, baseDir)
		return nil
	})

	// start goroutine to run docker ps -o wide
	g.Go(func() error {
		if err := semaphore.Acquire(ctx, 1); err != nil {
			return err
		}
		defer semaphore.Release(1)
		ch <- getServTop(ctx, container, baseDir)
		return nil
	})

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

func getServLog(ctx context.Context, container, baseDir string) *archiveFile {
	return archiveFileFromCommand(
		ctx,
		filepath.Join(baseDir, fmt.Sprintf("%v.log", container)),
		"docker", "container", "logs", container,
	)
}

func getServInspect(ctx context.Context, container, baseDir string) *archiveFile {
	return archiveFileFromCommand(
		ctx,
		filepath.Join(baseDir, fmt.Sprintf("inspect-%v.txt", container)),
		"docker", "container", "inspect", container,
	)
}

func getServTop(ctx context.Context, container, baseDir string) *archiveFile {
	return archiveFileFromCommand(
		ctx,
		filepath.Join(baseDir, fmt.Sprintf("top-%v.txt", container)),
		"docker", "top", container,
	)
}

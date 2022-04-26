package main

import (
	"archive/zip"
	"context"
	"flag"
	"fmt"
	"log"
	"os/exec"
	"path"
	"strings"
	"sync"
	"unicode"

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
		if strings.HasSuffix(base, ".zip") == false {
			baseDir = base
			base = base + ".zip"
		} else {
			baseDir = strings.TrimSuffix(base, ".zip")
		}

		ctx := context.Background()
		containers, err := getContainers(ctx)

		log.Printf("Archiving docker-cli data for %d containers\n SRC_ENDPOINT: %v\n Output filename: %v", len(containers), cfg.Endpoint, base)

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
TODO: handle for single container/server instance
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
			ch <- getContainerLog(ctx, container, baseDir)
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

	// start goroutine to get configs
	if configs == true {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch <- getExternalServicesConfig(ctx, baseDir)
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			ch <- getSiteConfig(ctx, baseDir)
		}()
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
	c, err := exec.CommandContext(ctx, "docker", "container", "ls", "--format", "{{.Names}} {{.Networks}}").Output()
	if err != nil {
		fmt.Errorf("failed to get container names with error: %w", err)
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

func getContainerLog(ctx context.Context, container, baseDir string) *archiveFile {
	return archiveFileFromCommand(ctx, baseDir, path.Join("/docker/containers/", container, container)+".log", "docker", "container", "logs", container)
}

//func getContainerLog(ctx context.Context, container, baseDir string) *archiveFile {
//	f := &archiveFile{name: baseDir + "/docker/containers/" + container + "/" + container + ".log"}
//	f.data, f.err = exec.CommandContext(ctx, "docker", "container", "logs", container).CombinedOutput()
//	return f
//}

func getInspect(ctx context.Context, container, baseDir string) *archiveFile {
	return archiveFileFromCommand(ctx, baseDir, path.Join("/docker/containers/", container, "/inspect-"+container)+".txt", "docker", "container", "inspect", container)
}

//func getInspect(ctx context.Context, container, baseDir string) *archiveFile {
//	f := &archiveFile{name: baseDir + "/docker/containers/" + container + "/inspect-" + container + ".txt"}
//	f.data, f.err = exec.CommandContext(ctx, "docker", "container", "inspect", container).CombinedOutput()
//	return f
//}

func getStats(ctx context.Context, baseDir string) *archiveFile {
	return archiveFileFromCommand(ctx, baseDir, "/docker/stats.txt", "docker", "container", "stats", "--no-stream")
}

//func getStats(ctx context.Context, baseDir string) *archiveFile {
//	f := &archiveFile{name: baseDir + "/docker/stats.txt"}
//	f.data, f.err = exec.CommandContext(ctx, "docker", "container", "stats", "--no-stream").CombinedOutput()
//	return f
//}

package main

import (
	"archive/zip"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/sourcegraph/sourcegraph/lib/errors"
	"github.com/sourcegraph/src-cli/internal/exec"
)

/*
General Stuff
TODO: file issue on the existence of OAuth signKey which needs to be redacted
TODO: Create getSiteConfig function
*/

type archiveFile struct {
	name string
	data []byte
	err  error
}

func archiveFileFromCommand(ctx context.Context, path, cmd string, args ...string) *archiveFile {
	f := &archiveFile{name: path}
	f.data, f.err = exec.CommandContext(ctx, cmd, args...).CombinedOutput()
	if f.err != nil {
		f.err = errors.Wrapf(f.err, "executing command: %s %s: received error: %s", cmd, strings.Join(args, " "), f.data)
	}
	return f
}

// This function prompts the user to confirm they want to run the command
func verify(confirmationText string) (bool, error) {
	input := ""
	for strings.ToLower(input) != "y" && strings.ToLower(input) != "n" {
		fmt.Printf("%s [y/N]: ", confirmationText)
		if _, err := fmt.Scanln(&input); err != nil {
			return false, err
		}
	}

	return strings.ToLower(input) == "y", nil
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

// write to archive all the outputs from kubectl call functions passed to buffer channel
func writeChannelContentsToZip(zw *zip.Writer, ch <-chan *archiveFile, verbose bool) error {
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

// TODO: Currently external services and site configs are pulled using the src endpoints

// getExternalServicesConfig calls src extsvc list with the format flag -f,
//and then returns an archiveFile to be consumed
func getExternalServicesConfig(ctx context.Context, baseDir string) *archiveFile {
	const fmtStr = `{{range .Nodes}}{{.id}} | {{.kind}} | {{.displayName}}{{"\n"}}{{.config}}{{"\n---\n"}}{{end}}`
	return archiveFileFromCommand(
		ctx,
		filepath.Join(baseDir, "config", "external_services.txt"),
		os.Args[0], "extsvc", "list", "-f", fmtStr,
	)
}

// getSiteConfig calls src api -query=... to query the api for site config json
// TODO: correctly format json output before writing to zip
func getSiteConfig(ctx context.Context, baseDir string) *archiveFile {
	const siteConfigStr = `query { site { configuration { effectiveContents } } }`
	return archiveFileFromCommand(ctx,
		filepath.Join(baseDir, "config", "siteConfig.json"),
		os.Args[0], "api", "-query", siteConfigStr,
	)
}

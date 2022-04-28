package main

import (
	"archive/zip"
	"context"
	"fmt"
	"os"
	"path"
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

// TODO: Currently external services and site configs are pulled using the src endpoints

// getExternalServicesConfig calls src extsvc list with the format flag -f,
//and then returns an archiveFile to be consumed
func getExternalServicesConfig(ctx context.Context, baseDir string) *archiveFile {
	const fmtStr = `{{range .Nodes}}{{.id}} | {{.kind}} | {{.displayName}}{{"\n"}}{{.config}}{{"\n---\n"}}{{end}}`
	return archiveFileFromCommand(
		ctx,
		path.Join(baseDir, "/config/external_services.txt"),
		os.Args[0], "extsvc", "list", "-f", fmtStr,
	)
}

//func getExternalServicesConfig(ctx context.Context, baseDir string) *archiveFile {
//	const fmtStr = `{{range .Nodes}}{{.id}} | {{.kind}} | {{.displayName}}{{"\n"}}{{.config}}{{"\n---\n"}}{{end}}`
//
//	f := &archiveFile{name: baseDir + "/config/external_services.txt"}
//	f.data, f.err = exec.CommandContext(ctx, os.Args[0], "extsvc", "list", "-f", fmtStr).CombinedOutput()
//
//	return f
//}

// getSiteConfig calls src api -query=... to query the api for site config json
// TODO: correctly format json output before writing to zip
func getSiteConfig(ctx context.Context, baseDir string) *archiveFile {
	const siteConfigStr = `query { site { configuration { effectiveContents } } }`
	return archiveFileFromCommand(ctx,
		path.Join(baseDir, "/config/siteConfig.json"),
		os.Args[0], "api", "-query", siteConfigStr,
	)
}

//func getSiteConfig(ctx context.Context, baseDir string) *archiveFile {
//	const siteConfigStr = `query { site { configuration { effectiveContents } } }`
//	f := &archiveFile{name: baseDir + "/config/siteConfig.json"}
//	f.data, f.err = exec.CommandContext(ctx, os.Args[0], "api", "-query", siteConfigStr).CombinedOutput()
//
//	return f
//}

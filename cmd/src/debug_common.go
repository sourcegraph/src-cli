package main

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/sourcegraph/src-cli/internal/exec"

	"github.com/sourcegraph/sourcegraph/lib/errors"
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

// write all the outputs from an archive command passed on the channel to to the zip writer
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

		if _, err := zf.Write(f.data); err != nil {
			return fmt.Errorf("failed to write to %s: %w", f.name, err)
		}
	}
	return nil
}

// TODO: Currently external services and site configs are pulled using the src endpoints

// getExternalServicesConfig calls src extsvc list with the format flag -f,
// and then returns an archiveFile to be consumed
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
	f := archiveFileFromCommand(ctx,
		filepath.Join(baseDir, "config", "siteConfig.json"),
		os.Args[0], "api", "-query", siteConfigStr,
	)

	if f.err != nil {
		return f
	}

	var siteConfig struct {
		Data struct {
			Site struct {
				Configuration struct {
					EffectiveContents string
				}
			}
		}
	}

	f.err = json.Unmarshal(f.data, &siteConfig)
	if f.err != nil {
		return f
	}

	f.data = []byte(siteConfig.Data.Site.Configuration.EffectiveContents)
	return f
}

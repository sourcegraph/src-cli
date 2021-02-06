package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/sourcegraph/src-cli/internal/api"

	"github.com/tj/go-update"
	"github.com/tj/go-update/progress"
	"github.com/tj/go-update/stores/github"
)

const (
	// PrivateFileMode grants owner to read/write a file.
	PrivateFileMode = 0600
)

func init() {
	usage := `
Examples:

	Update and replace src-cli to the recommended version:

		$ src update
`

	flagSet := flag.NewFlagSet("update", flag.ExitOnError)

	var apiFlags = api.NewFlags(flagSet)

	handler := func(args []string) error {
		currentDirectory, err := os.Getwd()
		if err != nil {
			return err
		}
		err = isDirWritable(currentDirectory)
		if err != nil {
			runWithElevatedPrivilege()
			return nil
		}

		fmt.Printf("Current version: %s\n", buildTag)

		client := cfg.apiClient(apiFlags, flagSet.Output())
		recommendedVersion, err := getRecommendedVersion(context.Background(), client)
		if err != nil {
			return err
		}
		if recommendedVersion == "" {
			fmt.Println("Recommended Version: <unknown>")
			return nil
		}
		if buildTag == recommendedVersion {
			fmt.Printf("src-cli is already at the recommended version: %s\n", recommendedVersion)
			return nil
		}

		// Retrieving latest release information.
		updateManager := &update.Manager{
			Command: "src",
			Store: &github.Store{
				Owner:   "sourcegraph",
				Repo:    "src-cli",
				Version: buildTag,
			},
		}
		if runtime.GOOS == "windows" {
			updateManager.Command += ".exe"
		}
		releases, err := updateManager.LatestReleases()
		if err != nil {
			return err
		}
		if len(releases) == 0 {
			return fmt.Errorf("No latest src-cli release update")
		}
		latestRelease := releases[0]
		if latestRelease.Version != recommendedVersion {
			return fmt.Errorf("Mismatch of recommended version")
		}

		updateAsset := latestRelease.FindTarball(runtime.GOOS, runtime.GOARCH)
		if updateAsset == nil {
			return fmt.Errorf("src-cli binary has not been installed in this machine")
		}
		fmt.Printf("Downloading src-cli version %s\n", recommendedVersion)
		tarball, err := updateAsset.DownloadProxy(progress.Reader)
		if err != nil {
			return fmt.Errorf("Failed downloading src-cli release: %v\n", err)
		}

		bin, err := exec.LookPath(updateManager.Command)
		if err != nil {
			return fmt.Errorf("Failed looking up path of %v\n", updateManager.Command)
		}
		dir := filepath.Dir(bin)

		if runtime.GOOS == "windows" {
			// Windows need to use absolute path for the file.
			if dir == "." {
				currDir, err := os.Getwd()
				if err != nil {
					return fmt.Errorf("Failed detecting current directory: %v\n", err)
				}
				dir = currDir
			}

			// Workaround for Windows to rename the old file first.
			dst := filepath.Join(dir, updateManager.Command)
			old := dst + ".old"
			if err := os.Rename(dst, old); err != nil {
				return fmt.Errorf("Windows renaming\n")
			}
		}

		if err := updateManager.InstallTo(tarball, dir); err != nil {
			return fmt.Errorf("Failed installing src-cli: %v\n", err)
		}

		fmt.Printf("src-cli has been updated from version %s to %s\n", buildTag, recommendedVersion)

		// Keep the terminal installation window open after Windows' UAC privilege elevation.
		if runtime.GOOS == "windows" {
			fmt.Println("Press Enter key to exit")
			fmt.Scanln()
		}

		return nil
	}

	// Register the command.
	commands = append(commands, &command{
		flagSet: flagSet,
		handler: handler,
		usageFunc: func() {
			fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src %s':\n", flagSet.Name())
			flagSet.PrintDefaults()
			fmt.Println(usage)
		},
	})
}

func isDirWritable(dir string) error {
	f := filepath.Join(dir, ".touch")
	if err := ioutil.WriteFile(f, []byte(""), PrivateFileMode); err != nil {
		return fmt.Errorf("Does not have write permission to %v directory: %v\n", dir, err)
	}
	return os.Remove(f)
}

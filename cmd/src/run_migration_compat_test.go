package main

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/urfave/cli/v3"

	"github.com/sourcegraph/src-cli/internal/clicompat"
)

// TestMaybeRunMigratedCommandAppliesColorMode verifies that migrated
// subcommands (e.g. `src version`) apply force16Color from config before
// running the command.
//
// The test injects a temporary no-op migrated command so we don't run real
// network handlers, points readConfig at a temp config file with
// force16Color enabled, drives maybeRunMigratedCommand, and asserts that
// the ansiColors table was remapped to the 16-color palette.
func TestMaybeRunMigratedCommandAppliesColorMode(t *testing.T) {
	snapshotAnsiColors(t)

	// 1. Inject a no-op migrated command so we don't run real handlers.
	const testCmd = "__test_color_noop"
	migratedCommands[testCmd] = clicompat.Wrap(&cli.Command{
		Name:        testCmd,
		HideHelp:    true,
		HideVersion: true,
		Action: func(ctx context.Context, c *cli.Command) error {
			return nil
		},
	})
	t.Cleanup(func() { delete(migratedCommands, testCmd) })

	// 2. Write a config file enabling force16Color.
	home := t.TempDir()
	cfgFile := filepath.Join(home, "src-config.json")
	data, err := json.Marshal(configFromFile{
		Endpoint:     "https://example.com",
		AccessToken:  "test-token",
		Force16Color: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgFile, data, 0o600); err != nil {
		t.Fatal(err)
	}
	oldConfigPath := *configPath
	*configPath = cfgFile
	t.Cleanup(func() { *configPath = oldConfigPath })

	// 3. Swap os.Args so flag.Parse() and runMigrated() see our command.
	oldArgs := os.Args
	os.Args = []string{"src", testCmd}
	t.Cleanup(func() { os.Args = oldArgs })

	// 4. Reset flag.CommandLine so the in-test flag.Parse picks up our
	//    os.Args without conflicting with previously registered flags.
	oldFlagSet := flag.CommandLine
	flag.CommandLine = flag.NewFlagSet(oldArgs[0], flag.ContinueOnError)
	t.Cleanup(func() { flag.CommandLine = oldFlagSet })

	// 5. Redirect stdout/stderr to /dev/null (or NUL on Windows) so
	//    incidental output from urfave/cli doesn't pollute test output.
	devnull, errOpen := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if errOpen == nil {
		oldStdout, oldStderr := os.Stdout, os.Stderr
		os.Stdout, os.Stderr = devnull, devnull
		t.Cleanup(func() {
			os.Stdout, os.Stderr = oldStdout, oldStderr
			devnull.Close()
		})
	}

	isMigrated, _, runErr := maybeRunMigratedCommand()
	if runErr != nil {
		t.Fatalf("maybeRunMigratedCommand returned error: %v", runErr)
	}
	if !isMigrated {
		t.Fatalf("expected %q to be detected as a migrated command", testCmd)
	}

	after := ansiColors["logo"]
	if strings.Contains(after, "38;5;") || strings.Contains(after, "48;5;") {
		t.Errorf(`migrated command path did not apply force16Color: ansiColors["logo"] = %q, still a 256-color SGR sequence`, after)
	}
}

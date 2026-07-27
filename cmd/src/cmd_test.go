package main

import (
	"encoding/json"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestCommanderRunAppliesColorMode verifies the legacy commander.run path,
// which calls os.Exit after running a command, in a subprocess.
func TestCommanderRunAppliesColorMode(t *testing.T) {
	snapshotAnsiColors(t)

	// Sanity precondition: the default "logo" entry must be a 256-color
	// SGR sequence, otherwise observing a remap is meaningless.
	if before := ansiColors["logo"]; !strings.Contains(before, "38;5;") {
		t.Fatalf(`precondition failed: ansiColors["logo"] = %q, expected a 256-color sequence`, before)
	}

	// 1. Write a config file enabling force16Color, point *configPath at it.
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

	child := exec.Command(os.Args[0], "-test.run=^TestCommanderRunAppliesColorModeChild$")
	child.Env = append(os.Environ(),
		"SRC_CLI_TEST_COMMANDER_FORCE16=1",
		"SRC_CLI_TEST_CONFIG="+cfgFile,
	)
	out, err := child.Output()
	if err != nil {
		t.Fatalf("child command failed: %v", err)
	}
	if got := string(out); strings.Contains(got, "38;5;") || strings.Contains(got, "48;5;") {
		t.Errorf(`commander.run did not apply force16Color before dispatching the subcommand handler: ansiColors["logo"] = %q, still a 256-color SGR sequence`, got)
	}
}

func TestCommanderRunAppliesColorModeChild(t *testing.T) {
	if os.Getenv("SRC_CLI_TEST_COMMANDER_FORCE16") != "1" {
		t.Skip("helper process")
	}
	*configPath = os.Getenv("SRC_CLI_TEST_CONFIG")

	const subName = "__test_legacy_color"
	c := commander{&command{
		flagSet: flag.NewFlagSet(subName, flag.ContinueOnError),
		handler: func(args []string) error {
			_, _ = os.Stdout.WriteString(ansiColors["logo"])
			return nil
		},
	}}
	c.run(flag.NewFlagSet("src", flag.ContinueOnError), "src", "usage", []string{subName})
}

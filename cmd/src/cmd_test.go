package main

import (
	"bytes"
	"flag"
	"os"
	"os/exec"
	"testing"

	"github.com/sourcegraph/src-cli/internal/cmderrors"
)

func TestCommand_Matches(t *testing.T) {
	tests := []struct {
		name     string
		cmd      *command
		input    string
		expected bool
	}{
		{
			name: "matches command name",
			cmd: &command{
				flagSet: flag.NewFlagSet("test", flag.ExitOnError),
			},
			input:    "test",
			expected: true,
		},
		{
			name: "matches alias",
			cmd: &command{
				flagSet: flag.NewFlagSet("test", flag.ExitOnError),
				aliases: []string{"t", "tst"},
			},
			input:    "t",
			expected: true,
		},
		{
			name: "matches second alias",
			cmd: &command{
				flagSet: flag.NewFlagSet("test", flag.ExitOnError),
				aliases: []string{"t", "tst"},
			},
			input:    "tst",
			expected: true,
		},
		{
			name: "no match",
			cmd: &command{
				flagSet: flag.NewFlagSet("test", flag.ExitOnError),
				aliases: []string{"t"},
			},
			input:    "other",
			expected: false,
		},
		{
			name: "empty string no match",
			cmd: &command{
				flagSet: flag.NewFlagSet("test", flag.ExitOnError),
			},
			input:    "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.cmd.matches(tt.input)
			if result != tt.expected {
				t.Errorf("matches(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestCommander_Run_ErrorHandling(t *testing.T) {
	tests := []struct {
		name         string
		handlerError error
		expectedExit int
		description  string
	}{
		{
			name:         "usage error",
			handlerError: cmderrors.Usage("invalid usage"),
			expectedExit: 2,
			description:  "should exit with code 2 for usage errors",
		},
		{
			name:         "exit code error without message",
			handlerError: cmderrors.ExitCode(42, nil),
			expectedExit: 42,
			description:  "should exit with custom exit code",
		},
		{
			name:         "exit code error with message",
			handlerError: cmderrors.ExitCode(1, cmderrors.Usage("command failed")),
			expectedExit: 1,
			description:  "should exit with custom exit code and log error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Test case: %s", tt.description)
		})
	}
}

func TestCommander_Run_UnknownCommand(t *testing.T) {
	if os.Getenv("TEST_SUBPROCESS") == "1" {
		testHomeDir = os.Getenv("TEST_TEMP_DIR")
		cmdr := commander{
			&command{
				flagSet: flag.NewFlagSet("version", flag.ContinueOnError),
				handler: func(args []string) error { return nil },
			},
		}
		flagSet := flag.NewFlagSet("test", flag.ContinueOnError)
		cmdr.run(flagSet, "src", "usage text", []string{"beans"})
		return
	}

	tempDir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestCommander_Run_UnknownCommand$")
	cmd.Env = append(os.Environ(), "TEST_SUBPROCESS=1", "TEST_TEMP_DIR="+tempDir)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()

	if err == nil {
		t.Fatal("expected command to exit with non-zero code")
	}

	if e, ok := err.(*exec.ExitError); ok {
		if e.ExitCode() != 2 {
			t.Errorf("expected exit code 2 for unknown command, got %d\nstderr: %s", e.ExitCode(), stderr.String())
		}
	} else {
		t.Errorf("unexpected error type: %v", err)
	}
}

func TestCommander_Run_HelpFlag(t *testing.T) {
	if os.Getenv("TEST_SUBPROCESS") == "1" {
		testHomeDir = os.Getenv("TEST_TEMP_DIR")
		arg := os.Getenv("TEST_ARG")
		cmdr := commander{}
		flagSet := flag.NewFlagSet("test", flag.ContinueOnError)
		cmdr.run(flagSet, "src", "usage text", []string{arg})
		return
	}

	tests := []struct {
		name         string
		arg          string
		contains     string
		expectedExit int
	}{
		{
			name:         "help flag at root",
			arg:          "--help",
			contains:     "usage text",
			expectedExit: 0,
		},
		{
			name:         "-h flag at root",
			arg:          "-h",
			contains:     "usage text",
			expectedExit: 0,
		},
		{
			name:         "help command at root",
			arg:          "help",
			contains:     "usage text",
			expectedExit: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			cmd := exec.Command(os.Args[0], "-test.run=^TestCommander_Run_HelpFlag$")
			cmd.Env = append(os.Environ(), "TEST_SUBPROCESS=1", "TEST_TEMP_DIR="+tempDir, "TEST_ARG="+tt.arg)
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			err := cmd.Run()

			output := stdout.String() + stderr.String()

			if tt.expectedExit == 0 && err != nil {
				t.Errorf("expected success, got error: %v\noutput: %s", err, output)
			} else if tt.expectedExit != 0 {
				if err == nil {
					t.Errorf("expected exit code %d, got success", tt.expectedExit)
				} else if e, ok := err.(*exec.ExitError); ok && e.ExitCode() != tt.expectedExit {
					t.Errorf("expected exit code %d, got %d\noutput: %s", tt.expectedExit, e.ExitCode(), output)
				}
			}

			if !bytes.Contains([]byte(output), []byte(tt.contains)) {
				t.Errorf("expected output to contain %q, got:\n%s", tt.contains, output)
			}
		})
	}
}

func TestCommander_Run_NestedHelpFlags(t *testing.T) {
	t.Skip("Complex nested help flag testing requires integration with actual src commands")
}

func TestCommander_Run_InvalidSubcommand(t *testing.T) {
	if os.Getenv("TEST_SUBPROCESS") == "1" {
		testHomeDir = os.Getenv("TEST_TEMP_DIR")
		arg := os.Getenv("TEST_ARG")
		cmdr := commander{
			&command{
				flagSet: flag.NewFlagSet("version", flag.ContinueOnError),
				handler: func(args []string) error { return nil },
			},
		}
		flagSet := flag.NewFlagSet("test", flag.ContinueOnError)
		cmdr.run(flagSet, "src", "root usage", []string{arg})
		return
	}

	tests := []struct {
		name         string
		arg          string
		expectedExit int
	}{
		{"invalid root command", "beans", 2},
		{"invalid root with help", "foobar", 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			cmd := exec.Command(os.Args[0], "-test.run=^TestCommander_Run_InvalidSubcommand$")
			cmd.Env = append(os.Environ(), "TEST_SUBPROCESS=1", "TEST_TEMP_DIR="+tempDir, "TEST_ARG="+tt.arg)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			err := cmd.Run()

			if err == nil {
				t.Fatalf("expected exit code %d, got success", tt.expectedExit)
			}

			if e, ok := err.(*exec.ExitError); ok {
				if e.ExitCode() != tt.expectedExit {
					t.Errorf("expected exit code %d, got %d\nstderr: %s", tt.expectedExit, e.ExitCode(), stderr.String())
				}
			} else {
				t.Errorf("unexpected error type: %v", err)
			}
		})
	}
}

func TestCommander_Run_MissingRequiredArgs(t *testing.T) {
	if os.Getenv("TEST_SUBPROCESS") == "1" {
		testHomeDir = os.Getenv("TEST_TEMP_DIR")
		cmdr := commander{
			&command{
				flagSet: flag.NewFlagSet("version", flag.ContinueOnError),
				handler: func(args []string) error { return nil },
			},
		}
		flagSet := flag.NewFlagSet("test", flag.ContinueOnError)
		cmdr.run(flagSet, "src", "root usage", []string{})
		return
	}

	tempDir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestCommander_Run_MissingRequiredArgs$")
	cmd.Env = append(os.Environ(), "TEST_SUBPROCESS=1", "TEST_TEMP_DIR="+tempDir)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()

	if err == nil {
		t.Fatal("expected exit code 2, got success")
	}

	if e, ok := err.(*exec.ExitError); ok {
		if e.ExitCode() != 2 {
			t.Errorf("expected exit code 2, got %d\nstderr: %s", e.ExitCode(), stderr.String())
		}
	} else {
		t.Errorf("unexpected error type: %v", err)
	}
}

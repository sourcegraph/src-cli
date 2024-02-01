package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/Masterminds/semver"
	"github.com/sourcegraph/sourcegraph/lib/errors"

	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/cmderrors"
	"github.com/sourcegraph/src-cli/internal/version"
)

// command is a subcommand handler and its flag set.
type command struct {
	// flagSet is the flag set for the command.
	flagSet *flag.FlagSet

	// aliases for the command.
	aliases []string

	// handler is the function that is invoked to handle this command.
	handler func(args []string) error

	// flagSet.Usage function to invoke on e.g. -h flag. If nil, a default one is
	// used.
	usageFunc func()
}

// matches tells if the given name matches this command or one of its aliases.
func (c *command) matches(name string) bool {
	if name == c.flagSet.Name() {
		return true
	}
	for _, alias := range c.aliases {
		if name == alias {
			return true
		}
	}
	return false
}

// commander represents a top-level command with subcommands.
type commander []*command

// run runs the command.
func (c commander) run(flagSet *flag.FlagSet, cmdName, usageText string, args []string) {
	// Parse flags.
	flagSet.Usage = func() {
		_, _ = fmt.Fprint(flag.CommandLine.Output(), usageText)
	}
	if !flagSet.Parsed() {
		_ = flagSet.Parse(args)
	}

	// Read global configuration.
	var err error
	cfg, err = readConfig()
	if err != nil {
		log.Fatal("reading config: ", err)
	}

	// Check and warn about outdated version.
	client := cfg.apiClient(api.NewFlags(flagSet), flagSet.Output())
	checkForOutdatedVersion(client)

	// Print usage if the command is "help".
	if flagSet.Arg(0) == "help" || flagSet.NArg() == 0 {
		flagSet.Usage()
		os.Exit(0)
	}

	// Configure default usage funcs for commands.
	for _, cmd := range c {
		cmd := cmd
		if cmd.usageFunc != nil {
			cmd.flagSet.Usage = cmd.usageFunc
			continue
		}
		cmd.flagSet.Usage = func() {
			_, _ = fmt.Fprintf(flag.CommandLine.Output(), "Usage of '%s %s':\n", cmdName, cmd.flagSet.Name())
			cmd.flagSet.PrintDefaults()
		}
	}

	// Find the subcommand to execute.
	name := flagSet.Arg(0)
	for _, cmd := range c {
		if !cmd.matches(name) {
			continue
		}

		// Parse subcommand flags.
		args := flagSet.Args()[1:]
		if err := cmd.flagSet.Parse(args); err != nil {
			panic(fmt.Sprintf("all registered commands should use flag.ExitOnError: error: %s", err))
		}

		// Execute the subcommand.
		if err := cmd.handler(flagSet.Args()[1:]); err != nil {
			if _, ok := err.(*cmderrors.UsageError); ok {
				log.Printf("error: %s\n\n", err)
				cmd.flagSet.Usage()
				os.Exit(2)
			}
			if e, ok := err.(*cmderrors.ExitCodeError); ok {
				if e.HasError() {
					log.Println(e)
				}
				os.Exit(e.Code())
			}
			log.Fatal(err)
		}
		os.Exit(0)
	}
	log.Printf("%s: unknown subcommand %q", cmdName, name)
	log.Fatalf("Run '%s help' for usage.", cmdName)
}

func didYouMeanOtherCommand(actual string, suggested []string) *command {
	fullSuggestions := make([]string, len(suggested))
	for i, s := range suggested {
		fullSuggestions[i] = "src " + s
	}
	msg := fmt.Sprintf("src: unknown subcommand %q\n\nDid you mean:\n\n\t%s", actual, strings.Join(fullSuggestions, "\n\t"))
	return &command{
		flagSet:   flag.NewFlagSet(actual, flag.ExitOnError),
		handler:   func(args []string) error { return errors.New(msg) },
		usageFunc: func() { log.Println(msg) },
	}
}

func checkForOutdatedVersion(client api.Client) {
	if version.BuildTag != version.DefaultBuildTag {
		recommendedVersion, err := getRecommendedVersion(context.Background(), client)
		if err != nil {
			log.Fatal("failed to get recommended version for Sourcegraph deployment: ", err)
		}
		if recommendedVersion == "" {
			log.Println("Recommended version: <unknown>\nThis Sourcegraph instance does not support this feature.")
		} else {
			constraints, err := semver.NewConstraint(fmt.Sprintf("<=%s", version.BuildTag))
			if err != nil {
				log.Fatal("failed to check current version: ", err)
			}

			recommendedVersionInstance, err := semver.NewVersion(recommendedVersion)
			if err != nil {
				log.Fatal("failed to check version returned by Sourcegraph: ", err)
			}

			if !constraints.Check(recommendedVersionInstance) {
				log.Printf("⚠️  You are using an outdated version %s. Please upgrade to %s or later.\n", version.BuildTag, recommendedVersion)
			}
		}
	}
}

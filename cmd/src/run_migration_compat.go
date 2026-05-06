package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"

	"github.com/sourcegraph/src-cli/internal/clicompat"
	"github.com/sourcegraph/src-cli/internal/cmderrors"
	"github.com/urfave/cli/v3"

	"github.com/sourcegraph/sourcegraph/lib/errors"
)

var migratedCommands = map[string]*cli.Command{
	"abc":     abcCommand,
	"api":     apiCommand,
	"auth":    authCommand,
	"login":   loginCommand,
	"version": versionCommand,
}

func maybeRunMigratedCommand() (isMigrated bool, exitCode int, err error) {
	// need to figure out if a migrated command has been requested
	flag.Parse()
	subCommand := flag.CommandLine.Arg(0)
	_, isMigrated = migratedCommands[subCommand]
	if !isMigrated {
		return
	}
	cfg, err = readConfig()
	if err != nil {
		log.Fatal("reading config: ", err)
	}

	exitCode, err = runMigrated()
	return
}

// migratedRootCommand constructs a root 'src' command and adds
// MigratedCommands as subcommands to it
func migratedRootCommand() *cli.Command {
	names := make([]string, 0, len(migratedCommands))
	for name := range migratedCommands {
		names = append(names, name)
	}
	sort.Strings(names)

	commands := make([]*cli.Command, 0, len(names))
	for _, name := range names {
		commands = append(commands, migratedCommands[name])
	}

	return clicompat.Wrap(&cli.Command{
		Name:        "src",
		HideVersion: true,
		Commands:    commands,
	})
}

// runMigrated runs the command within urfave/cli framework
func runMigrated() (int, error) {
	ctx := context.Background()

	err := migratedRootCommand().Run(ctx, os.Args)
	if err != nil {
		if errors.HasType[*cmderrors.UsageError](err) {
			return 2, nil
		}
		if e, ok := err.(*cmderrors.ExitCodeError); ok {
			if e.HasError() {
				return e.Code(), e
			}
			return e.Code(), nil
		}
		var exitErr cli.ExitCoder
		if errors.AsInterface(err, &exitErr) {
			return exitErr.ExitCode(), err
		}

		return 1, err
	}

	return 0, err
}

// runLegacy runs the command using the original commander framework
func runLegacy(cmd *command, flagSet *flag.FlagSet) (int, error) {
	// Parse subcommand flags.
	args := flagSet.Args()[1:]
	if err := cmd.flagSet.Parse(args); err != nil {
		fmt.Printf("Error parsing subcommand flags: %s\n", err)
		panic(fmt.Sprintf("all registered commands should use flag.ExitOnError: error: %s", err))
	}

	// Execute the subcommand.
	if err := cmd.handler(flagSet.Args()[1:]); err != nil {
		if _, ok := err.(*cmderrors.UsageError); ok {
			log.Printf("error: %s\n\n", err)
			cmd.flagSet.SetOutput(os.Stderr)
			flag.CommandLine.SetOutput(os.Stderr)
			cmd.flagSet.Usage()
			return 2, nil
		}
		if e, ok := err.(*cmderrors.ExitCodeError); ok {
			if e.HasError() {
				log.Println(e)
			}
			return e.Code(), nil
		}
		return 1, err
	}
	return 0, nil
}

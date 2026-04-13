package main

import (
	"context"
	stderrors "errors"
	"flag"
	"fmt"
	"log"
	"os"
	"slices"
	"sort"

	"github.com/sourcegraph/src-cli/internal/clicompat"
	"github.com/sourcegraph/src-cli/internal/cmderrors"
	"github.com/urfave/cli/v3"

	"github.com/sourcegraph/sourcegraph/lib/errors"
)

var MigratedCommands = map[string]*cli.Command{
	"version": versionCommandv2,
}

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
	return slices.Contains(c.aliases, name)
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

	// Print usage if the command is "help".
	if flagSet.Arg(0) == "help" || flagSet.NArg() == 0 {
		flagSet.SetOutput(os.Stdout)
		flagSet.Usage()
		os.Exit(0)
	}

	// Configure default usage funcs for commands.
	for _, cmd := range c {
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
		_, isMigratedCmd := MigratedCommands[name]
		if !isMigratedCmd && !cmd.matches(name) {
			continue
		}
		// Read global configuration now.
		var err error
		cfg, err = readConfig()
		if err != nil {
			log.Fatal("reading config: ", err)
		}

		var exitCode int

		if isMigratedCmd {
			exitCode, err = runMigrated(flagSet)
		} else {
			exitCode, err = runLegacy(cmd, flagSet)
		}
		if err != nil {
			log.Fatal(err)
		}
		os.Exit(exitCode)

	}
	log.Printf("%s: unknown subcommand %q", cmdName, name)
	log.Fatalf("Run '%s help' for usage.", cmdName)
}

// migratedRootCommand constructs a root 'src' command and adds
// MigratedCommands as subcommands to it
func migratedRootCommand() *cli.Command {
	names := make([]string, 0, len(MigratedCommands))
	for name := range MigratedCommands {
		names = append(names, name)
	}
	sort.Strings(names)

	commands := make([]*cli.Command, 0, len(names))
	for _, name := range names {
		commands = append(commands, MigratedCommands[name])
	}

	return clicompat.WrapRoot(&cli.Command{
		Name:        "src",
		HideVersion: true,
		Commands:    commands,
	})
}

// runMigrated runs the command within urfave/cli framework
func runMigrated(flagSet *flag.FlagSet) (int, error) {
	ctx := context.Background()
	args := append([]string{"src"}, flagSet.Args()...)

	err := migratedRootCommand().Run(ctx, args)
	if _, ok := stderrors.AsType[*cmderrors.UsageError](err); ok {
		return 2, nil
	}
	var exitErr cli.ExitCoder
	if errors.AsInterface(err, &exitErr) {
		return exitErr.ExitCode(), err
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

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"slices"

	"github.com/sourcegraph/src-cli/internal/cmderrors"
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

	// Check if --help args are anywhere in the command; if yes, then
	// remove it from the list of args at this point to avoid interrupting recursive function calls,
	// and append it to the deepest command / subcommand
	filteredArgs := make([]string, 0, len(args))
	helpRequested := false

	helpFlags := []string{
		"--h",
		"--help",
		"-h",
		"-help",
		"help",
	}

	for _, arg := range args {
		if slices.Contains(helpFlags, arg) {
			helpRequested = true
		} else {
			filteredArgs = append(filteredArgs, arg)
		}
	}

	// Parse flags.
	flagSet.Usage = func() {
		_, _ = fmt.Fprint(flag.CommandLine.Output(), usageText)
	}
	if !flagSet.Parsed() {
		_ = flagSet.Parse(filteredArgs)
	}

	// If no subcommands remain (or help requested with no subcommands), print usage.
	if flagSet.NArg() == 0 {
		flagSet.SetOutput(os.Stdout)
		flagSet.Usage()
		os.Exit(0)
	}

	// Configure default usage funcs for commands.
	for _, cmd := range c {
		cmd := cmd

		// If the command / subcommand has defined its own usageFunc, then use it
		if cmd.usageFunc != nil {
			cmd.flagSet.Usage = cmd.usageFunc
			continue
		}

		// If the command / subcommand has not defined its own usageFunc,
		// then generate a basic default usageFunc,
		// using the command's defined flagSet and their defaults
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

		// Read global configuration now.
		var err error
		cfg, err = readConfig()
		if err != nil {
			log.Fatal("reading config: ", err)
		}

		// Get subcommand args, and re-add help flag if it was requested
		args := flagSet.Args()[1:]
		if helpRequested {
			args = append(args, "-h")
		}

		// Set output to stdout for help (flag package defaults to stderr)
		cmd.flagSet.SetOutput(os.Stdout)
		flag.CommandLine.SetOutput(os.Stdout)

		// Execute the subcommand
		if err := cmd.handler(args); err != nil {
			if _, ok := err.(*cmderrors.UsageError); ok {
				log.Printf("error: %s\n\n", err)
				cmd.flagSet.SetOutput(os.Stderr)
				flag.CommandLine.SetOutput(os.Stderr)
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

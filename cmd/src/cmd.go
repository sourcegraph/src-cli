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

// Run the command
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

	// Define the usage function from usageText
	flagSet.Usage = func() {
		_, _ = fmt.Fprint(flag.CommandLine.Output(), usageText)
	}

	// Parse the command's flags, if not already parsed
	if !flagSet.Parsed() {
		_ = flagSet.Parse(filteredArgs)
	}

	// If no subcommands remain (or help requested with no subcommands), print usage
	if flagSet.NArg() == 0 {
		flagSet.SetOutput(os.Stdout)
		flagSet.Usage()
		os.Exit(0)
	}

	// Configure default usage funcs for all commands.
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

	// Find the subcommand to execute
	// Assume the subcommand is the first arg in the flagSet
	name := flagSet.Arg(0)

	// Loop through the list of all registered subcommands
	for _, cmd := range c {

		// If the first arg is not this registered commmand in the loop, try the next registered command
		if !cmd.matches(name) {
			continue
		}
		// If the first arg is this registered commmand in the loop, then try and run it, then exit

		// Read global configuration
		var err error
		cfg, err = readConfig()
		if err != nil {
			log.Fatal("reading config: ", err)
		}

		// Get the remaining args, to pass to the subcommand, as an unparsed array of previously parsed args
		args := flagSet.Args()[1:]

		// Set output to stdout for help (flag package defaults to stderr)
		cmd.flagSet.SetOutput(os.Stdout)
		flag.CommandLine.SetOutput(os.Stdout)

		// Note: We can't parse flags here because commanders need to pass unparsed args to subcommand handlers
		// Each handler is responsible for parsing its own flags
		// All commands must use `flagSet := flag.NewFlagSet("<name>", flag.ExitOnError)` to ensure usage helper text is printed automatically on arg parse errors
		// Parse the subcommand's args, on its behalf, to test if flag.ExitOnError is not set
		// if err := cmd.flagSet.Parse(args); err != nil {
		// 	fmt.Printf("Error parsing subcommand flags: %s\n", err)
		// 	panic(fmt.Sprintf("all registered commands should use flag.ExitOnError: error: %s", err))
		// }

		// If the --help arg was provided, re-add it here for the lowest command to parse and action
		if helpRequested {
			args = append(args, "-h")
		}

		// Execute the subcommand
		if err := cmd.handler(args); err != nil {

			// If the subcommand returns a UsageError, then print the error and usage helper text
			if _, ok := err.(*cmderrors.UsageError); ok {
				log.Printf("error: %s\n\n", err)
				cmd.flagSet.SetOutput(os.Stderr)
				flag.CommandLine.SetOutput(os.Stderr)
				cmd.flagSet.Usage()
				os.Exit(2)
			}

			// If the subcommand returns any other error, then print the error
			if e, ok := err.(*cmderrors.ExitCodeError); ok {
				if e.HasError() {
					log.Println(e)
				}
				os.Exit(e.Code())
			}
			log.Fatal(err)
		}

		// If no error was returned, then exit the application
		os.Exit(0)
	}

	// If the first arg didn't match any registered commands, print errors and exit the application
	log.Printf("%s: unknown subcommand %q", cmdName, name)
	log.Fatalf("Run '%s help' for usage.", cmdName)
}

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

	// NOTE: This function is quite brittle
	// Especially with printing helper text at all 3 different levels of depth

	// Check if --help args are anywhere in the command
	// If yes, then remove it from the list of args at this point,
	// then append it to the deepest command / subcommand, later,
	// to avoid outputting usage text for a commander when a subcommand is specified
	filteredArgs := make([]string, 0, len(args))
	helpRequested := false

	helpFlags := []string{
		"help",
		"-help",
		"--help",
		"-h",
		"--h",
	}

	for _, arg := range args {
		if slices.Contains(helpFlags, arg) {
			helpRequested = true
		} else {
			filteredArgs = append(filteredArgs, arg)
		}
	}

	// Define the usage function for the commander
	flagSet.Usage = func() {
		_, _ = fmt.Fprint(flag.CommandLine.Output(), usageText)
	}

	// Parse the commander's flags, if not already parsed
	if !flagSet.Parsed() {
		_ = flagSet.Parse(filteredArgs)
	}

	// Find the subcommand to execute
	// This assumes the subcommand is the first arg in the flagSet,
	// i.e. any global args have been removed from the flagSet
	name := flagSet.Arg(0)

	// Loop through the list of all registered subcommands
	for _, cmd := range c {

		// If the first arg is not this registered commmand in the loop, try the next registered command
		if !cmd.matches(name) {
			continue
		}
		// If the first arg is this registered commmand in the loop, then try and run it, then exit

		// Set up the usage function for this subcommand
		if cmd.usageFunc != nil {
			// If the subcommand has a usageFunc defined, then use it
			cmd.flagSet.Usage = cmd.usageFunc
		} else {
			// If the subcommand does not have a usageFunc defined,
			// then define a simple default one,
			// using the list of flags defined in the subcommand, and their description strings
			cmd.flagSet.Usage = func() {
				_, _ = fmt.Fprintf(flag.CommandLine.Output(), "Usage of '%s %s':\n", cmdName, cmd.flagSet.Name())
				cmd.flagSet.PrintDefaults()
			}
		}

		// Read global configuration
		var err error
		cfg, err = readConfig()
		if err != nil {
			log.Fatal("reading config: ", err)
		}

		// Get the remainder of the args, excluding the first arg / this command name
		args := flagSet.Args()[1:]

		// Set output to stdout, for usage / helper text printed for the --help flag (flag package defaults to stderr)
		cmd.flagSet.SetOutput(os.Stdout)
		flag.CommandLine.SetOutput(os.Stdout)

		// If the --help arg was provided, re-add it here for the lowest command to parse and action
		if helpRequested {
			args = append(args, "-h")
		}

		// Parse the subcommand's args, on its behalf, to test and ensure flag.ExitOnError is set
		// just in case any future authors of subcommands forget to set flag.ExitOnError
		if err := cmd.flagSet.Parse(args); err != nil {
			fmt.Printf("Error parsing subcommand flags: %s\n", err)
			panic(fmt.Sprintf("all registered commands should use flag.ExitOnError: error: %s", err))
		}

		// Execute the subcommand
		// Handle any errors returned
		if err := cmd.handler(args); err != nil {

			// If the returned error is of type UsageError
			if _, ok := err.(*cmderrors.UsageError); ok {
				// then print the error and usage helper text, both to stderr
				log.Printf("error: %s\n\n", err)
				cmd.flagSet.SetOutput(os.Stderr)
				flag.CommandLine.SetOutput(os.Stderr)
				cmd.flagSet.Usage()
				os.Exit(2)
			}

			// If the returned error is of type ExitCodeError
			if e, ok := err.(*cmderrors.ExitCodeError); ok {
				// Then log the error and exit with the exit code
				if e.HasError() {
					log.Println(e)
				}
				os.Exit(e.Code())
			}

			// For all other types of errors, log them as fatal, and exit
			log.Fatal(err)
		}

		// If no error was returned, then just exit the application cleanly
		os.Exit(0)
	}

	// To make it after the big loop, that means name didn't match any registered commands
	if name != "" {
		log.Printf("%s: unknown command %q", cmdName, name)
		flagSet.Usage()
		os.Exit(2)
	}

	// Special case to handle --help usage text for src command
	if helpRequested {
		// Set output to stdout, for usage / helper text printed for the --help flag (flag package defaults to stderr)
		flagSet.SetOutput(os.Stdout)
		flagSet.Usage()
		os.Exit(0)
	}

	// Special case to handle src command with no args
	flagSet.Usage()
	os.Exit(2)

}

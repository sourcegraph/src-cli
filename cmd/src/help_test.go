package main

import (
	"sort"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// registeredRootCommandNames computes the set of visible top-level command
// names straight from the two registries, independently of rootCommands(), so
// the tests below catch a command that is registered but left out of the help
// text or the docs.
func registeredRootCommandNames() []string {
	seen := map[string]bool{}
	for _, cmd := range commands {
		if !cmd.hidden {
			seen[cmd.flagSet.Name()] = true
		}
	}
	for _, cmd := range migratedCommands {
		if !cmd.Hidden {
			seen[cmd.Name] = true
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// helpCommandNames parses the "The commands are:" block of the given 'src
// help' output and returns the command names (without aliases), sorted.
func helpCommandNames(t *testing.T, help string) []string {
	t.Helper()

	_, block, ok := strings.Cut(help, "The commands are:\n")
	if !ok {
		t.Fatalf("help text has no \"The commands are:\" block:\n%s", help)
	}
	block, _, _ = strings.Cut(block, "\nUse \"src [command] -h\"")

	var names []string
	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		name, _, _ := strings.Cut(line, " ")
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func TestHelpListsAllRegisteredCommands(t *testing.T) {
	got := helpCommandNames(t, usageText())
	if diff := cmp.Diff(registeredRootCommandNames(), got); diff != "" {
		t.Errorf("'src help' command list does not match the registered commands (-registered +help):\n%s", diff)
	}
}

func TestHelpHidesHiddenCommands(t *testing.T) {
	help := usageText()
	for _, cmd := range commands {
		if cmd.hidden && strings.Contains(help, "\t"+cmd.flagSet.Name()+" ") {
			t.Errorf("hidden command %q is listed in 'src help'", cmd.flagSet.Name())
		}
	}
	if !strings.Contains(help, "\tabc ") || !strings.Contains(help, "\tbatch") {
		t.Errorf("expected both a urfave/cli command (abc) and a legacy command (batch) in help:\n%s", help)
	}
}

func TestRootCommandsAreWellFormed(t *testing.T) {
	names := map[string]bool{}
	for _, cmd := range rootCommands() {
		if cmd.description == "" {
			t.Errorf("command %q has no description: set description on the legacy command or Usage on the urfave/cli command", cmd.name)
		}
		if names[cmd.name] {
			t.Errorf("command %q is registered more than once", cmd.name)
		}
		names[cmd.name] = true
		for _, alias := range cmd.aliases {
			if alias == cmd.name {
				t.Errorf("command %q lists its own name as an alias", cmd.name)
			}
		}
	}
	for _, cmd := range rootCommands() {
		for _, alias := range cmd.aliases {
			if names[alias] {
				t.Errorf("alias %q of command %q collides with another command's name", alias, cmd.name)
			}
		}
	}
}

func TestFormatCommandList(t *testing.T) {
	got := formatCommandList([]rootCommand{
		{name: "a", description: "first"},
		{name: "longer", aliases: []string{"l", "lg"}, description: "second"},
	})
	want := "\ta       first\n" +
		"\tlonger  second (alias: l, lg)\n"
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("formatCommandList mismatch (-want +got):\n%s", diff)
	}
}

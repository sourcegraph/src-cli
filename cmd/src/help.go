package main

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/sourcegraph/sourcegraph/lib/docgen"
)

// rootCommand is a top-level 'src' command as shown in 'src help'. It is the
// single source for the command list in the help text and for the tests that
// keep 'src help' and the 'src doc' root index in sync.
type rootCommand struct {
	name        string
	aliases     []string
	description string
}

// rootCommands returns every visible top-level command, whether it is
// registered with the legacy commander (commands) or with urfave/cli
// (migratedCommands), sorted by name.
func rootCommands() []rootCommand {
	var root []rootCommand

	for _, cmd := range commands {
		if cmd.hidden {
			continue
		}
		name := cmd.flagSet.Name()
		var aliases []string
		for _, alias := range cmd.aliases {
			// Some legacy commands register their own name as an alias.
			if alias != name {
				aliases = append(aliases, alias)
			}
		}
		root = append(root, rootCommand{
			name:        name,
			aliases:     aliases,
			description: cmd.description,
		})
	}

	for _, cmd := range docgen.VisibleCommands(migratedRootCommand().Commands) {
		root = append(root, rootCommand{
			name:        cmd.Name,
			aliases:     slices.Clone(cmd.Aliases),
			description: cmd.Usage,
		})
	}

	slices.SortFunc(root, func(a, b rootCommand) int {
		return cmp.Compare(a.name, b.name)
	})
	return root
}

// formatCommandList renders the "The commands are:" block of 'src help':
// one tab-indented line per command with the name padded to a common width,
// the description, and any aliases in parentheses.
func formatCommandList(cmds []rootCommand) string {
	width := 0
	for _, cmd := range cmds {
		width = max(width, len(cmd.name))
	}

	var b strings.Builder
	for _, cmd := range cmds {
		fmt.Fprintf(&b, "\t%-*s  %s", width, cmd.name, cmd.description)
		if len(cmd.aliases) > 0 {
			fmt.Fprintf(&b, " (alias: %s)", strings.Join(cmd.aliases, ", "))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// usageText renders the top-level 'src help' output.
func usageText() string {
	return usageHeader + formatCommandList(rootCommands()) + usageFooter
}

const usageHeader = `src is a tool that provides access to Sourcegraph instances.
For more information, see https://github.com/sourcegraph/src-cli

Usage:

	src [options] command [command options]

Environment variables
	SRC_ACCESS_TOKEN  Sourcegraph access token
	SRC_ENDPOINT      endpoint to use, if unset will default to "https://sourcegraph.com"
	SRC_PROXY         A proxy to use for proxying requests to the Sourcegraph endpoint.
	                  Supports HTTP(S), SOCKS5/5h, and UNIX Domain Socket proxies.
					  If a UNIX Domain Socket, the path can be either an absolute path,
					  or can start with ~/ or %USERPROFILE%\ for a path in the user's home directory.
					  Examples:
						- https://localhost:3080
						- https://<user>:<password>localhost:8080
						- socks5h://localhost:1080
						- socks5://<username>:<password>@localhost:1080
						- unix://~/src-proxy.sock
						- unix://%USERPROFILE%\src-proxy.sock
						- ~/src-proxy.sock
						- %USERPROFILE%\src-proxy.sock
						- C:\some\path\src-proxy.sock

The options are:

	-v                               print verbose output

The commands are:

`

const usageFooter = `
Use "src [command] -h" for more information about a command.

`

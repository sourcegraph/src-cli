package main

import (
	"bytes"
	"cmp"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"path"
	"slices"
	"strings"
	"text/template"

	"github.com/urfave/cli/v3"

	"github.com/sourcegraph/sourcegraph/lib/docgen"
	"github.com/sourcegraph/sourcegraph/lib/errors"
	"github.com/sourcegraph/sourcegraph/lib/output"

	"github.com/sourcegraph/src-cli/internal/cmderrors"
	"github.com/sourcegraph/src-cli/internal/version"
)

func init() {
	flagSet := flag.NewFlagSet("doc", flag.ExitOnError)
	var (
		outputFlag = flagSet.String("o", "", "Output directory, or output file with -json")
		jsonFlag   = flagSet.Bool("json", false, "Write the reference as one JSON file instead of Markdown files")
	)

	usage := `
'src doc' is an internal command that generates the Markdown or JSON reference
documentation used within Sourcegraph.

Usage:

	src doc -o DIR
	src doc -json -o FILE

Examples:

    $ src doc -o ~/sourcegraph/doc/integration/cli/reference

    $ src doc -json -o ~/sourcegraph/docs/cli/references.json
	`

	handler := func(args []string) error {
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		out := output.NewOutput(flagSet.Output(), output.OutputOpts{Verbose: *verbose})
		if *outputFlag == "" {
			out.WriteLine(output.Line(output.EmojiFailure, output.StyleWarning, "output must be set via -o"))
			flagSet.Usage()
			return cmderrors.ExitCode(1, nil)
		}

		dstDir := *outputFlag

		dr, err := newDocRenderer()
		if err != nil {
			return err
		}

		commanders := map[string]*commander{
			"":            &commands,
			"batch":       &batchCommands,
			"config":      &configCommands,
			"extsvc":      &extsvcCommands,
			"code-intel":  &codeintelCommands,
			"repos":       &reposCommands,
			"search-jobs": &searchJobsCommands,
			"debug":       &debugCommands,
			"snapshot":    &snapshotCommands,
		}

		if *jsonFlag {
			// src has two kinds of commands: legacy flag.FlagSet commands, listed
			// in commanders, and urfave/cli commands in migratedCommands.
			root := &docCommand{Name: "src"}
			if version.BuildTag != version.DefaultBuildTag {
				root.Version = version.BuildTag
			}
			if root.Commands, err = legacyDocCommands(commanders, ""); err != nil {
				return err
			}
			for _, cmd := range docgen.VisibleCommands(migratedRootCommand().Commands) {
				root.Commands = append(root.Commands, migratedDocCommand("src "+cmd.Name, cmd))
			}
			sortDocCommands(root.Commands)
			var buf bytes.Buffer
			enc := json.NewEncoder(&buf)
			enc.SetEscapeHTML(false)
			enc.SetIndent("", "  ")
			if err := enc.Encode(root); err != nil {
				return err
			}
			content, err := sanitiseDocText(buf.String())
			if err != nil {
				return err
			}
			return os.WriteFile(*outputFlag, []byte(content), 0644)
		}

		rootSubcommands := map[string]string{}
		count := 0
		for groupName, cmdr := range commanders {
			subcommands := map[string]string{}

			for _, cmd := range *cmdr {
				name := cmd.flagSet.Name()

				// Figure out the fully qualified name of this command.
				fqcn := strings.TrimSpace(strings.Join([]string{
					groupName,
					name,
				}, " "))

				if cmd.hidden {
					continue
				}

				// If this name appears in our commanders map, then this isn't a
				// real command, and we'll handle it differently.
				if _, ok := commanders[fqcn]; !ok {
					content, err := dr.RenderCommand(fqcn, cmd)
					if err != nil {
						return err
					}

					file, err := openDocFile(dstDir, fqcn)
					if err != nil {
						return err
					}
					defer file.Close()

					if _, err := file.WriteString(content); err != nil {
						return err
					}
					count++
					subcommands[name] = name + ".md"
				} else {
					subcommands[name] = name + "/index.md"
				}
			}

			if groupName == "" {
				maps.Copy(rootSubcommands, subcommands)
				continue
			}

			content, err := dr.RenderGroup(groupName, subcommands)
			if err != nil {
				return err
			}

			file, err := openDocFile(dstDir, groupName+" index")
			if err != nil {
				return err
			}
			defer file.Close()

			if _, err := file.WriteString(content); err != nil {
				return err
			}
			count++
		}

		root := migratedRootCommand()
		mdFiles, err := docgen.Markdown(root)
		if err != nil {
			return err
		}

		for _, cmd := range docgen.VisibleCommands(root.Commands) {
			rootSubcommands[cmd.Name] = docgen.SubcommandDocPath(cmd)
		}

		for _, md := range mdFiles {
			out.Verbosef("writing: %s", md.Name)
			docPath := path.Join(dstDir, md.Name)
			if err := os.MkdirAll(path.Dir(docPath), 0755); err != nil {
				return err
			}
			if err := os.WriteFile(docPath, []byte(md.Content), 0644); err != nil {
				return err
			}
			count++
		}

		content, err := dr.RenderGroup("", rootSubcommands)
		if err != nil {
			return err
		}

		file, err := openDocFile(dstDir, " index")
		if err != nil {
			return err
		}
		defer file.Close()

		if _, err := file.WriteString(content); err != nil {
			return err
		}
		count++
		out.VerboseLine(output.Linef(output.EmojiSuccess, output.StyleSuccess, "%d files rendered under %s", count, *outputFlag))

		return nil
	}

	commands = append(commands, &command{
		flagSet: flagSet,
		hidden:  true,
		handler: handler,
		usageFunc: func() {
			fmt.Fprintln(flag.CommandLine.Output(), usage)
		},
	})
}

// docCommand is one command in the reference that 'src doc -json' writes: a
// single tree of all visible commands, like gcloud's CLI tree. Only name is
// required. Only the root has a version: the src-cli release that wrote it.
// Flags are the command's own visible flags; hidden commands are skipped, and
// commands and flags are sorted by name.
type docCommand struct {
	Name     string        `json:"name"`
	Version  string        `json:"version,omitempty"`
	Summary  string        `json:"summary,omitempty"`
	Usage    string        `json:"usage,omitempty"`
	Help     string        `json:"help,omitempty"`
	Flags    []docFlag     `json:"flags,omitempty"`
	Commands []*docCommand `json:"commands,omitempty"`
}

type docFlag struct {
	Name        string   `json:"name"`
	Aliases     []string `json:"aliases,omitempty"`
	Default     string   `json:"default,omitempty"`
	Description string   `json:"description,omitempty"`
}

func sortDocCommands(commands []*docCommand) {
	slices.SortFunc(commands, func(a, b *docCommand) int { return cmp.Compare(a.Name, b.Name) })
}

// legacyDocCommands returns the group's visible legacy commands. Their help is
// the free text that their usage function prints.
func legacyDocCommands(commanders map[string]*commander, group string) ([]*docCommand, error) {
	var commands []*docCommand
	for _, cmd := range *commanders[group] {
		if cmd.hidden {
			continue
		}
		fqcn := strings.TrimSpace(group + " " + cmd.flagSet.Name())

		// Most usage functions print "Usage of 'src ...':" and the flags
		// before the help. Leave both out: the flags are listed separately.
		cmd.flagSet.SetOutput(io.Discard)
		usage, err := captureUsage(cmd)
		cmd.flagSet.SetOutput(nil)
		if err != nil {
			return nil, err
		}
		usage = strings.TrimSpace(usage)
		if first, rest, _ := strings.Cut(usage, "\n"); strings.HasPrefix(first, "Usage of ") {
			usage = strings.Trim(rest, "\n")
		}

		c := &docCommand{Name: "src " + fqcn, Summary: cmd.description, Help: usage}
		cmd.flagSet.VisitAll(func(f *flag.Flag) {
			c.Flags = append(c.Flags, docFlag{Name: f.Name, Default: f.DefValue, Description: f.Usage})
		})
		if _, ok := commanders[fqcn]; ok {
			if c.Commands, err = legacyDocCommands(commanders, fqcn); err != nil {
				return nil, err
			}
		}
		commands = append(commands, c)
	}
	sortDocCommands(commands)
	return commands, nil
}

// migratedDocCommand returns the urfave/cli command and its visible
// subcommands.
func migratedDocCommand(name string, cmd *cli.Command) *docCommand {
	c := &docCommand{
		Name:    name,
		Summary: cmd.Usage,
		Usage:   strings.TrimSpace(cmd.UsageText),
		Help:    strings.TrimSpace(cmd.Description),
	}
	for _, f := range cmd.Flags {
		df := f.(cli.DocGenerationFlag)
		c.Flags = append(c.Flags, docFlag{Name: f.Names()[0], Aliases: f.Names()[1:], Default: df.GetValue(), Description: df.GetUsage()})
	}
	slices.SortFunc(c.Flags, func(a, b docFlag) int { return cmp.Compare(a.Name, b.Name) })
	for _, sub := range docgen.VisibleCommands(cmd.Commands) {
		c.Commands = append(c.Commands, migratedDocCommand(name+" "+sub.Name, sub))
	}
	return c
}

func openDocFile(base, fqcn string) (interface {
	io.StringWriter
	io.WriteCloser
}, error) {
	cmdPath := path.Join(append([]string{base}, strings.Split(fqcn, " ")...)...)
	if err := os.MkdirAll(path.Dir(cmdPath), 0755); err != nil {
		return nil, err
	}

	return os.Create(cmdPath + ".md")
}

// sanitiseDocText replaces the user's directories in v, such as in flag
// defaults, with generic ones.
func sanitiseDocText(v string) (string, error) {
	// Just replacing the home directory will probably cover off most
	// cases, but we'll do the user cache and config as well just to be
	// safe.
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", errors.Wrap(err, "getting user cache dir")
	}

	config, err := os.UserConfigDir()
	if err != nil {
		return "", errors.Wrap(err, "getting user config dir")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", errors.Wrap(err, "getting home dir")
	}

	v = strings.ReplaceAll(v, cache, "~/.cache")
	v = strings.ReplaceAll(v, config, "~/.config")
	v = strings.ReplaceAll(v, home, "~")

	return v, nil
}

type docRenderer struct {
	commandTemplate *template.Template
	groupTemplate   *template.Template
}

func newDocRenderer() (*docRenderer, error) {
	ct := template.New("command")
	ct = ct.Funcs(template.FuncMap{
		"sanitise": sanitiseDocText,
	})

	ct, err := ct.Parse(docCommandTemplate)
	if err != nil {
		return nil, err
	}

	gt, err := template.New("group").Parse(docGroupTemplate)
	if err != nil {
		return nil, err
	}

	return &docRenderer{
		commandTemplate: ct,
		groupTemplate:   gt,
	}, nil
}

func (dr *docRenderer) RenderCommand(fqcn string, cmd *command) (string, error) {
	flags := []*flag.Flag{}
	cmd.flagSet.VisitAll(func(f *flag.Flag) {
		flags = append(flags, f)
	})

	usage, err := captureUsage(cmd)
	if err != nil {
		return "", err
	}

	buf := &bytes.Buffer{}
	if err := dr.commandTemplate.Execute(buf, &docCommandContext{
		FQCN:  fqcn,
		Usage: usage,
		Flags: flags,
	}); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// captureUsage returns the output of the command's usage function.
func captureUsage(cmd *command) (string, error) {
	// The usage functions are, unfortunately, a bit of a mess right now. Some
	// output to flag.CommandLine.Output(), some to os.Stderr, some to
	// os.Stdout. So let's replace the stdout and stderr variables temporarily
	// to capture whatever output we get, then we can put everything back after.
	r, w, err := os.Pipe()
	if err != nil {
		return "", err
	}
	stdout := os.Stdout
	stderr := os.Stderr
	os.Stdout = w
	os.Stderr = w
	defer func() {
		os.Stdout = stdout
		os.Stderr = stderr
	}()
	out := make(chan string)
	go func() {
		buf := &bytes.Buffer{}
		if _, err := io.Copy(buf, r); err != nil {
			// This shouldn't ever panic in normal operation.
			panic(err)
		}
		out <- buf.String()
	}()
	if cmd.usageFunc != nil {
		cmd.usageFunc()
	} else if cmd.flagSet.Usage != nil {
		cmd.flagSet.Usage()
	}
	w.Close()
	return <-out, nil
}

func (dr *docRenderer) RenderGroup(fqcn string, commands map[string]string) (string, error) {
	buf := &bytes.Buffer{}
	if err := dr.groupTemplate.Execute(buf, &docGroupContext{
		FQCN:     fqcn,
		Commands: commands,
	}); err != nil {
		return "", err
	}

	return buf.String(), nil
}

type docCommandContext struct {
	FQCN  string
	Usage string
	Flags []*flag.Flag
}

type docGroupContext struct {
	FQCN     string
	Commands map[string]string
}

const (
	docCommandTemplate = `# ` + "`" + `src {{ .FQCN }}` + "`" + `

{{ if .Flags }}
## Flags

| Name | Description | Default Value |
|------|-------------|---------------|
{{- range .Flags -}}
{{- "\n" -}}
| ` + "`" + `-{{ .Name }}` + "`" + ` | {{ .Usage }} | {{ if .DefValue }}` + "`" + `{{ sanitise .DefValue }}` + "`" + `{{ end }} |
{{- end }}
{{ end }}

## Usage

` + "```" + `
{{ sanitise .Usage }}
` + "```" + `
	`

	docGroupTemplate = `# ` + "`" + `src{{ if .FQCN }} {{ .FQCN }}{{ end }}` + "`" + `

## Subcommands

{{ range $name, $link := .Commands -}}
{{- "\n" -}}
* [` + "`" + `{{ $name }}` + "`" + `]({{ $link }})
{{- end }}
	`
)

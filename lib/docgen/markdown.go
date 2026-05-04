package docgen

import (
	"bytes"
	"cmp"
	"fmt"
	"io"
	"path"
	"slices"
	"sort"
	"strings"
	"text/template"

	"github.com/urfave/cli/v3"

	"github.com/sourcegraph/sourcegraph/lib/errors"
)

type MarkdownFile struct {
	Name    string
	Content string
}

// Markdown renders a Markdown reference for the app.
//
// It is adapted from https://sourcegraph.com/github.com/urfave/cli-docs/-/blob/docs.go?L16
func Markdown(root *cli.Command) ([]MarkdownFile, error) {
	files := make([]MarkdownFile, 0, len(root.Commands))
	var errs error
	for _, sub := range VisibleCommands(root.Commands) {
		subFiles, err := markdownFiles(root.Name, []string{sub.Name}, sub)
		if err != nil {
			errs = errors.Append(errs, err)
		}
		files = append(files, subFiles...)
	}
	return files, errs
}

type cliTemplate struct {
	Title       string
	Usage       string
	Description string
	UsageText   string
	Flags       []flagRow
	Subcommands []subcommand
}

type subcommand struct {
	Name string
	Link string
}

type flagRow struct {
	Name    string
	Desc    string
	Default string
}

// markdownFiles recursively walks over cmd commands and sub commands to build a list of MarkdownFiles
// that contain the name and content for a command
func markdownFiles(rootName string, lineage []string, cmd *cli.Command) ([]MarkdownFile, error) {
	var w bytes.Buffer
	err := writeDocTemplate(rootName, lineage, cmd, &w)

	files := []MarkdownFile{{
		Name:    docPath(lineage, hasVisibleCommands(cmd.Commands)),
		Content: w.String(),
	}}

	for _, sub := range VisibleCommands(cmd.Commands) {
		subFiles, subErr := markdownFiles(rootName, append(lineage, sub.Name), sub)
		if subErr != nil {
			err = errors.Append(err, subErr)
		}
		files = append(files, subFiles...)
	}

	return files, err
}

func writeDocTemplate(rootName string, lineage []string, cmd *cli.Command, w io.Writer) error {
	const name = "cli"
	t, err := template.New(name).Parse(markdownDocTemplate)
	if err != nil {
		return err
	}

	title := strings.Join(append([]string{rootName}, lineage...), " ")
	return t.ExecuteTemplate(w, name, &cliTemplate{
		Title:       title,
		Usage:       prepareUsage(cmd),
		Description: strings.TrimSpace(cmd.Description),
		UsageText:   prepareUsageText(title, cmd),
		Flags:       prepareArgsWithValues(cmd.Flags),
		Subcommands: prepareSubcommands(cmd.Commands),
	})
}

func prepareSubcommands(commands []*cli.Command) []subcommand {
	links := make([]subcommand, 0, len(commands))
	for _, command := range VisibleCommands(commands) {
		links = append(links, subcommand{
			Name: command.Name,
			Link: SubcommandDocPath(command),
		})
	}
	return links
}

func prepareArgsWithValues(flags []cli.Flag) []flagRow {
	return prepareFlags(flags)
}

func prepareFlags(
	flags []cli.Flag,
) []flagRow {
	rows := []flagRow{}
	for _, f := range flags {
		flag, ok := f.(cli.DocGenerationFlag)
		if !ok {
			continue
		}
		names := make([]string, 0, len(f.Names()))
		for _, s := range f.Names() {
			trimmed := strings.TrimSpace(s)
			if trimmed == "" {
				continue
			}
			if len(trimmed) > 1 {
				names = append(names, fmt.Sprintf("--%s", trimmed))
			} else {
				names = append(names, fmt.Sprintf("-%s", trimmed))
			}
		}

		name := strings.Join(names, ", ")
		if len(name) > 0 {
			rows = append(rows, flagRow{
				Name:    name,
				Desc:    flag.GetUsage(),
				Default: flag.GetValue(),
			})
		}

	}
	slices.SortFunc(rows, func(a, b flagRow) int {
		return cmp.Compare(a.Name, b.Name)
	})
	return rows
}

func prepareUsageText(lineage string, command *cli.Command) string {
	if command.UsageText == "" {
		if hasVisibleCommands(command.Commands) {
			return renderUsageBlock(lineage + " [command options]")
		}
		if len(command.Flags) > 0 {
			return renderUsageBlock(lineage + " [options]")
		}
		if strings.TrimSpace(command.ArgsUsage) != "" {
			return fmt.Sprintf("Arguments: `%s`\n", command.ArgsUsage)
		}
		return ""
	}

	// Write all usage examples as a big shell code block.
	lines := make([]string, 0, strings.Count(command.UsageText, "\n")+1)
	for line := range strings.SplitSeq(strings.TrimSpace(command.UsageText), "\n") {
		line = strings.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		lines = append(lines, line)
	}
	return renderUsageBlock(lines...)
}

func renderUsageBlock(lines ...string) string {
	var usageText strings.Builder
	usageText.WriteString("```sh")
	for _, line := range lines {
		usageText.WriteByte('\n')

		if strings.HasPrefix(line, "# ") {
			usageText.WriteString(line)
		} else if len(line) > 0 {
			fmt.Fprintf(&usageText, "$ %s", line)
		}
	}
	usageText.WriteString("\n```\n")

	return usageText.String()
}

func prepareUsage(command *cli.Command) string {
	if command.Usage == "" {
		return ""
	}

	return command.Usage + "."
}

// VisibleCommands returns the non-hidden commands sorted by name.
func VisibleCommands(commands []*cli.Command) []*cli.Command {
	visible := make([]*cli.Command, 0, len(commands))
	for _, command := range commands {
		if command.Hidden {
			continue
		}
		visible = append(visible, command)
	}

	sort.Slice(visible, func(i, j int) bool {
		return visible[i].Name < visible[j].Name
	})

	return visible
}

// SubcommandDocPath returns the relative doc path for a direct child command.
func SubcommandDocPath(command *cli.Command) string {
	return docPath([]string{command.Name}, hasVisibleCommands(command.Commands))
}

func hasVisibleCommands(commands []*cli.Command) bool {
	for _, command := range commands {
		if !command.Hidden {
			return true
		}
	}
	return false
}

func docPath(lineage []string, isGroup bool) string {
	if len(lineage) == 0 {
		return "index.md"
	}
	if isGroup {
		return path.Join(path.Join(lineage...), "index.md")
	}
	if len(lineage) == 1 {
		return lineage[0] + ".md"
	}
	return path.Join(path.Join(lineage[:len(lineage)-1]...), lineage[len(lineage)-1]+".md")
}

var markdownDocTemplate = `# ` + "`" + `{{ .Title }}` + "`" + `

{{ if .Usage }}{{ .Usage }}

{{ end }}{{ if .Description }}{{ .Description }}
{{- end }}

{{ if .UsageText }}## Usage

{{ .UsageText }}
{{- end }}
{{ if .Flags }}## Flags

| Name | Description | Default Value |
|------|-------------|---------------|
{{- range .Flags -}}
{{- "\n" -}}
| ` + "`" + `{{ .Name }}` + "`" + ` | {{ .Desc }} | ` + "`" + `{{ .Default }}` + "`" + `|
{{- end }}{{- end }}
{{- if .Subcommands }}## Subcommands

{{ range $v := .Subcommands }}* [` + "`" + `{{ $v.Name }}` + "`" + `]({{ $v.Link }})
{{ end }}{{ end }}`

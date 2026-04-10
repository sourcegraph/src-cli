package clicompat

import "github.com/urfave/cli/v3"

// LegacyCommandHelpTemplate formats leaf command help in a style closer to the
// existing flag.FlagSet-based help output.
const LegacyCommandHelpTemplate = `Usage of '{{.FullName}}':
{{range .VisibleFlags}} {{printf "  -%s\n\t\t\t\t%s\n" .Name .Usage}}{{end}}{{if .Description}}
  {{trim .Description}}
{{end}}
`

// LegacyRootCommandHelpTemplate formats root command help while preserving a
// command's UsageText when it is provided.
const LegacyRootCommandHelpTemplate = `{{if .UsageText}}{{trim .UsageText}}
{{else}}Usage of '{{.FullName}}':
{{end}}{{if .VisibleFlags}}{{range .VisibleFlags}}{{println .}}{{end}}{{end}}{{if .VisibleCommands}}
{{range .VisibleCommands}}{{printf "\t%s\t%s\n" .Name .Usage}}{{end}}{{end}}{{if .Description}}
{{trim .Description}}
{{end}}
`

// WithLegacyCommandHelp applies the shared legacy-style leaf command help
// template and returns the same command for inline construction.
func WithLegacyCommandHelp(cmd *cli.Command) *cli.Command {
	if cmd == nil {
		return nil
	}

	cmd.CustomHelpTemplate = LegacyCommandHelpTemplate
	return cmd
}

// WithLegacyRootCommandHelp applies the shared legacy-style root help template
// and returns the same command for inline construction.
func WithLegacyRootCommandHelp(cmd *cli.Command) *cli.Command {
	if cmd == nil {
		return nil
	}

	cmd.CustomRootCommandHelpTemplate = LegacyRootCommandHelpTemplate
	return cmd
}

// WithLegacyHelp applies both root and leaf legacy help templates.
func WithLegacyHelp(cmd *cli.Command) *cli.Command {
	if cmd == nil {
		return nil
	}

	WithLegacyCommandHelp(cmd)
	WithLegacyRootCommandHelp(cmd)
	return cmd
}

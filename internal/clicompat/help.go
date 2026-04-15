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

// Wrap sets common options on a sub commands to ensure consistency for help and error handling
func Wrap(cmd *cli.Command) *cli.Command {
	if cmd == nil {
		return nil
	}

	cmd.CustomHelpTemplate = LegacyCommandHelpTemplate
	cmd.OnUsageError = OnUsageError
	return cmd
}

// WrapRoot sets common options on a root command to ensure consistency for help and error handling
func WrapRoot(cmd *cli.Command) *cli.Command {
	if cmd == nil {
		return nil
	}

	cmd.CustomRootCommandHelpTemplate = LegacyRootCommandHelpTemplate
	cmd.OnUsageError = OnUsageError
	return cmd
}

// WithLegacyHelp applies both root and leaf legacy help templates.
func WithLegacyHelp(cmd *cli.Command) *cli.Command {
	if cmd == nil {
		return nil
	}

	cmd.CustomHelpTemplate = LegacyCommandHelpTemplate
	cmd.CustomRootCommandHelpTemplate = LegacyRootCommandHelpTemplate
	cmd.OnUsageError = OnUsageError

	return cmd
}

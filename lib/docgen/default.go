package docgen

import (
	"bytes"

	"github.com/urfave/cli/v3"
)

// Default renders help text for the app using urfave/cli's default help format.
func Default(cmd *cli.Command) (string, error) {
	tpl := cmd.CustomRootCommandHelpTemplate
	if tpl == "" {
		tpl = cli.RootCommandHelpTemplate
	}

	var w bytes.Buffer
	cli.HelpPrinterCustom(&w, tpl, cmd, nil)
	return w.String(), nil
}

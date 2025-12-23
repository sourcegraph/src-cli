package main

import (
	"flag"
	"fmt"
	"io"

	"github.com/sourcegraph/src-cli/internal/lsp"
)

func init() {
	usage := `
'src lsp' runs a Language Server Protocol server that proxies LSP
requests to Sourcegraph's code intelligence backend.

The server communicates over stdio (stdin/stdout) and is designed
to be used with editors like Neovim.

Prerequisites:
  - The working directory must be inside a Git repository
  - The repository must be indexed on your Sourcegraph instance
  - SRC_ENDPOINT and SRC_ACCESS_TOKEN environment variables must be set

Supported LSP methods:
  - textDocument/definition
  - textDocument/references
  - textDocument/hover

Example Neovim configuration (0.11+):

  vim.lsp.config['src-lsp'] = {
    cmd = { 'src', 'lsp' },
    root_markers = { '.git' },
    filetypes = { 'go', 'typescript', 'python' },
  }
  vim.lsp.enable('src-lsp')
`

	flagSet := flag.NewFlagSet("lsp", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}

	handler := func(args []string) error {
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		client := cfg.apiClient(nil, io.Discard)

		srv, err := lsp.NewServer(client)
		if err != nil {
			return err
		}

		return srv.Run()
	}

	commands = append(commands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}

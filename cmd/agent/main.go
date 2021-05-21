package main

import (
	"context"
	"fmt"
	"os"

	"github.com/alexflint/go-arg"
)

var args = Args{}

type command interface {
	Execute(ctx context.Context, args *Args) error
}

func main() {
	parser := arg.MustParse(&args)
	ctx := context.Background()

	if err := args.dispatch(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		if err == errNoCommand {
			parser.WriteHelp(os.Stderr)
		}
		os.Exit(1)
	}
}

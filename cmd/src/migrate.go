package main

import (
	"context"
	"flag"
	"fmt"
	"os/exec"
	"strings"

	"github.com/sourcegraph/sourcegraph/lib/errors"
)

type genericData struct {
	data []byte
	err  error
}

func runDockerCommand(ctx context.Context, cmd string, args ...string) *genericData {
	f := &genericData{}

	f.data, f.err = exec.Command(cmd, args...).CombinedOutput()
	if f.err != nil {
		f.err = errors.Wrapf(f.err, "executing command: %s %s: received error: %s", cmd, strings.Join(args, " "), f.data)
	}
	return f
}

func init() {
	usage := `'src migrate' helps you migrate your sourcegraph data to another instance.

Usage:

    TBD

Examples:

	TBD
`

	var container string
	var outputPath string

	flagSet := flag.NewFlagSet("migrate", flag.ExitOnError)
	flagSet.StringVar(&container, "c", "", "The container to target")
	flagSet.StringVar(&outputPath, "o", "", "The path for the output file")

	handler := func(args []string) error {
		fmt.Fprintf(flag.CommandLine.Output(), "From container: %s'", container)

		// init context
		ctx := context.Background()

		data := runDockerCommand(
			ctx,
			"docker", "exec", container, "sh", "-c", "'pg_dump -C --username=postgres sourcegraph' > "+outputPath,
		)
		if data.err != nil {
			fmt.Println("Error:", data.err)
		}

		return nil
	}

	// Register the command.
	commands = append(commands, &command{
		flagSet: flagSet,
		aliases: []string{"migrate"},
		handler: handler,
		usageFunc: func() {
			fmt.Println(usage)
		},
	})
}

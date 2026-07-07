package pgdump

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sourcegraph/sourcegraph/lib/errors"
)

// Targets represents configuration for each of Sourcegraph's databases.
type Targets struct {
	Pgsql        Target `yaml:"pgsql"`
	CodeIntel    Target `yaml:"codeintel"`
	CodeInsights Target `yaml:"codeinsights"`
}

// Target represents a database for pg_dump to export.
type Target struct {
	// Target is the DSN of the database deployment:
	//
	// - in docker, the name of the database container, e.g. pgsql, codeintel-db, codeinsights-db
	// - in k8s, the name of the deployment or statefulset, e.g. deploy/pgsql, sts/pgsql
	// - in plain pg_dump, the server host or socket directory
	Target string `yaml:"target"`

	DBName   string `yaml:"dbname"`
	Username string `yaml:"username"`

	// Only include password if non-sensitive
	Password string `yaml:"password"`
}

// Command represents a command to run as an argv array rather than a shell
// string. Values that originate from user-controlled targets are always passed
// as discrete arguments (or safely shell-quoted when they must be embedded in a
// nested shell command), never interpolated into a string that is handed to
// "bash -c". This prevents shell injection via malicious targets files.
type Command struct {
	// Env holds additional environment variables in "KEY=value" form.
	Env []string
	// Args is the argv to execute; Args[0] is the executable.
	Args []string
	// InputFile, if set, is opened and connected to the command's stdin.
	InputFile string
	// OutputFile, if set, receives the command's stdout. When empty, stdout is
	// discarded (matching the original "1>/dev/null" behaviour for restores).
	OutputFile string
}

// Run executes the command safely (without a shell), wiring up any input/output
// file redirection, and returns the combined stderr (and stdout when it is not
// redirected to a file) output.
func (c Command) Run() ([]byte, error) {
	if len(c.Args) == 0 {
		return nil, errors.New("no command to run")
	}

	cmd := exec.Command(c.Args[0], c.Args[1:]...)
	cmd.Env = append(os.Environ(), c.Env...)

	var combined bytes.Buffer
	cmd.Stderr = &combined

	if c.InputFile != "" {
		f, err := os.Open(c.InputFile)
		if err != nil {
			return nil, errors.Wrapf(err, "opening input file %q", c.InputFile)
		}
		defer f.Close()
		cmd.Stdin = f
	}

	if c.OutputFile != "" {
		f, err := os.Create(c.OutputFile)
		if err != nil {
			return nil, errors.Wrapf(err, "creating output file %q", c.OutputFile)
		}
		defer f.Close()
		cmd.Stdout = f
	} else {
		// No output file means stdout is not of interest (e.g. restores), so
		// capture it alongside stderr for diagnostics.
		cmd.Stdout = &combined
	}

	err := cmd.Run()
	return combined.Bytes(), err
}

// String renders the command as a copy-pasteable, safely shell-quoted string.
// It is intended for display only; execution always goes through Run.
func (c Command) String() string {
	var parts []string
	for _, e := range c.Env {
		parts = append(parts, shellQuoteEnv(e))
	}
	for _, a := range c.Args {
		parts = append(parts, shellQuote(a))
	}
	s := strings.Join(parts, " ")
	if c.OutputFile != "" {
		s += " > " + shellQuote(c.OutputFile)
	}
	if c.InputFile != "" {
		s += " < " + shellQuote(c.InputFile)
	}
	return s
}

// RestoreCommand generates a psql invocation that can be used for migrations.
func RestoreCommand(t Target) (args []string, env []string) {
	args = []string{
		"psql",
		"--username=" + t.Username,
		"--dbname=" + t.DBName,
	}
	if t.Password != "" {
		env = append(env, "PGPASSWORD="+t.Password)
	}
	return args, env
}

// DumpCommand generates a pg_dump invocation that can be used for
// on-prem-to-Cloud migrations.
func DumpCommand(t Target) (args []string, env []string) {
	args = []string{
		"pg_dump",
		"--clean",
		"--format=plain",
		"--if-exists",
		"--no-acl",
		"--no-owner",
		"--quote-all-identifiers",
		"--username=" + t.Username,
		"--dbname=" + t.DBName,
	}
	if t.Password != "" {
		env = append(env, "PGPASSWORD="+t.Password)
	}
	return args, env
}

type Output struct {
	Output string
	Target Target
}

// Outputs generates a set of mappings between a pgdump.Target and the desired output
// path. It can be provided a zero-value Targets to just generate the output paths.
func Outputs(dir string, targets Targets) []Output {
	return []Output{{
		Output: filepath.Join(dir, "pgsql.sql"),
		Target: targets.Pgsql,
	}, {
		Output: filepath.Join(dir, "codeintel.sql"),
		Target: targets.CodeIntel,
	}, {
		Output: filepath.Join(dir, "codeinsights.sql"),
		Target: targets.CodeInsights,
	}}
}

type CommandBuilder func(Target) (Command, error)

// PGCommand generates the argv and environment for a base Postgres command
// (pg_dump or psql) targeting the given Target.
type PGCommand func(Target) (args []string, env []string)

// Builder generates the CommandBuilder and targetKey for a given builder and PGCommand
func Builder(builder string, command PGCommand) (commandBuilder CommandBuilder, targetKey string) {
	switch builder {
	case "pg_dump", "":
		targetKey = "local"
		commandBuilder = func(t Target) (Command, error) {
			args, env := command(t)
			if t.Target != "" {
				args = append(args, "--host="+t.Target)
			}
			return Command{Env: env, Args: args}, nil
		}
	case "docker":
		targetKey = "docker"
		commandBuilder = func(t Target) (Command, error) {
			args, env := command(t)
			inner := shellCommand(env, args)
			return Command{Args: []string{"docker", "exec", "-i", t.Target, "sh", "-c", inner}}, nil
		}
	case "kubectl":
		targetKey = "k8s"
		commandBuilder = func(t Target) (Command, error) {
			args, env := command(t)
			inner := shellCommand(env, args)
			return Command{Args: []string{"kubectl", "exec", "-i", t.Target, "--", "bash", "-c", inner}}, nil
		}
	default:
		return commandBuilder, targetKey
	}
	return commandBuilder, targetKey
}

// BuildCommands generates commands that output Postgres dumps and sends them to predefined
// files for each target database.
func BuildCommands(outDir string, commandBuilder CommandBuilder, targets Targets, dump bool) ([]Command, error) {
	var commands []Command
	for _, t := range Outputs(outDir, targets) {
		c, err := commandBuilder(t.Target)
		if err != nil {
			return nil, errors.Wrapf(err, "generating command for %q", t.Output)
		}

		if dump {
			// When dumping, redirect command stdout to the target file.
			c.OutputFile = t.Output
		} else {
			// When restoring, feed the target file to the command's stdin.
			c.InputFile = t.Output
		}
		commands = append(commands, c)
	}
	return commands, nil
}

// shellCommand renders env and args as a single, safely shell-quoted string for
// use as the argument to a nested "sh -c"/"bash -c" (e.g. inside a container).
func shellCommand(env, args []string) string {
	var parts []string
	for _, e := range env {
		parts = append(parts, shellQuoteEnv(e))
	}
	for _, a := range args {
		parts = append(parts, shellQuote(a))
	}
	return strings.Join(parts, " ")
}

// shellQuoteEnv quotes a "KEY=value" assignment, leaving the KEY= prefix
// unquoted (so the shell still treats it as an assignment) and quoting only the
// value.
func shellQuoteEnv(assignment string) string {
	i := strings.IndexByte(assignment, '=')
	if i < 0 {
		return shellQuote(assignment)
	}
	return assignment[:i+1] + shellQuote(assignment[i+1:])
}

// shellQuote returns s quoted such that a POSIX shell will treat it as a single
// literal argument.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	safe := true
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			continue
		}
		switch r {
		case '-', '_', '.', '/', ':', '=', '@', '%', '+', ',':
			continue
		}
		safe = false
		break
	}
	if safe {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

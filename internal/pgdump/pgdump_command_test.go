package pgdump

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// maliciousTarget is a target whose fields contain shell metacharacters,
// mirroring the injection payloads described in VULN-100.
func maliciousTarget() Target {
	return Target{
		Target:   "localhost; touch /tmp/pwned #",
		DBName:   "sg; rm -rf /",
		Username: "sg`whoami`",
		Password: "sg$(id)",
	}
}

func TestLocalCommandNoInjection(t *testing.T) {
	builder, key := Builder("pg_dump", DumpCommand)
	require.Equal(t, "local", key)

	cmd, err := builder(maliciousTarget())
	require.NoError(t, err)

	// Every untrusted value must be carried as a discrete argv element, never
	// merged into another token where the shell could reinterpret it.
	assert.Contains(t, cmd.Args, "--host=localhost; touch /tmp/pwned #")
	assert.Contains(t, cmd.Args, "--dbname=sg; rm -rf /")
	assert.Contains(t, cmd.Args, "--username=sg`whoami`")
	assert.Contains(t, cmd.Env, "PGPASSWORD=sg$(id)")

	// pg_dump is invoked directly, not via a shell.
	assert.Equal(t, "pg_dump", cmd.Args[0])
}

func TestDockerCommandNoInjection(t *testing.T) {
	builder, key := Builder("docker", DumpCommand)
	require.Equal(t, "docker", key)

	cmd, err := builder(maliciousTarget())
	require.NoError(t, err)

	// The target is passed as a discrete argv element to docker, so it cannot
	// break out into the calling shell.
	assert.Equal(t, []string{"docker", "exec", "-i"}, cmd.Args[:3])
	assert.Equal(t, "localhost; touch /tmp/pwned #", cmd.Args[3])
	assert.Equal(t, []string{"sh", "-c"}, cmd.Args[4:6])

	// The nested "sh -c" command embeds values, but they are shell-quoted so
	// they cannot alter control flow inside the container.
	inner := cmd.Args[6]
	assert.Contains(t, inner, `'--dbname=sg; rm -rf /'`)
	assert.Contains(t, inner, "'--username=sg`whoami`'")
	assert.Contains(t, inner, `PGPASSWORD='sg$(id)'`)
}

func TestKubectlCommandNoInjection(t *testing.T) {
	builder, key := Builder("kubectl", RestoreCommand)
	require.Equal(t, "k8s", key)

	cmd, err := builder(maliciousTarget())
	require.NoError(t, err)

	assert.Equal(t, []string{"kubectl", "exec", "-i"}, cmd.Args[:3])
	assert.Equal(t, "localhost; touch /tmp/pwned #", cmd.Args[3])
	assert.Equal(t, []string{"--", "bash", "-c"}, cmd.Args[4:7])

	inner := cmd.Args[7]
	assert.Contains(t, inner, `'--dbname=sg; rm -rf /'`)
}

func TestBuildCommandsRedirection(t *testing.T) {
	builder, _ := Builder("pg_dump", DumpCommand)
	targets := Targets{Pgsql: Target{DBName: "sg", Username: "sg"}}

	dumps, err := BuildCommands("out", builder, targets, true)
	require.NoError(t, err)
	require.NotEmpty(t, dumps)
	// Dump commands write to the output file; nothing is read from stdin.
	assert.Equal(t, "out/pgsql.sql", dumps[0].OutputFile)
	assert.Empty(t, dumps[0].InputFile)

	restores, err := BuildCommands("out", builder, targets, false)
	require.NoError(t, err)
	require.NotEmpty(t, restores)
	// Restore commands read from the input file; no shell redirection is used.
	assert.Equal(t, "out/pgsql.sql", restores[0].InputFile)
	assert.Empty(t, restores[0].OutputFile)
}

func TestCommandStringQuotesMaliciousValues(t *testing.T) {
	builder, _ := Builder("pg_dump", DumpCommand)
	cmd, err := builder(maliciousTarget())
	require.NoError(t, err)
	cmd.OutputFile = "out/pgsql.sql"

	s := cmd.String()
	// The rendered (display-only) string quotes dangerous tokens so that even a
	// copy-paste into a shell would not execute the injected commands.
	assert.False(t, strings.Contains(s, "; touch /tmp/pwned # >"),
		"unquoted injection leaked into rendered command: %s", s)
	assert.Contains(t, s, `'--host=localhost; touch /tmp/pwned #'`)
}

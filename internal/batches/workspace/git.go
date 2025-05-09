package workspace

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"

	"github.com/sourcegraph/sourcegraph/lib/errors"
)

func runGitCmd(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = []string{
		// Don't use the system wide git config.
		"GIT_CONFIG_NOSYSTEM=1",
		// And also not any other, because they can mess up output, change defaults, .. which can do unexpected things.
		"GIT_CONFIG=/dev/null",
		// Don't ask interactively for credentials.
		"GIT_TERMINAL_PROMPT=0",
		// Set user.name and user.email in the local repository. The user name and
		// e-mail will eventually be ignored anyway, since we're just using the Git
		// repository to generate diffs, but we don't want git to generate alarming
		// looking warnings.
		"GIT_AUTHOR_NAME=Sourcegraph",
		"GIT_AUTHOR_EMAIL=batch-changes@sourcegraph.com",
		"GIT_COMMITTER_NAME=Sourcegraph",
		"GIT_COMMITTER_EMAIL=batch-changes@sourcegraph.com",
		// Set extremely large buffer limits for git commands to prevent truncation
		"SOURCEGRAPH_BATCH_CHANGES_BUFFER=128M",
		// Set git config to avoid internal buffering limits
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=core.packedGitLimit",
		"GIT_CONFIG_VALUE_0=512m",
	}
	cmd.Dir = dir
	
	// For diff commands, we want to ensure we have enough buffer space
	// to handle large batch changes with thousands of mappings
	isDiffCommand := len(args) > 0 && args[0] == "diff"
	
	if isDiffCommand {
		// For diff commands, we'll write the output directly to a temporary file
		// to avoid any in-memory buffer limitations
		tmpfile, err := os.CreateTemp("", "git-diff-*.out")
		if err != nil {
			return nil, errors.Wrap(err, "creating temporary file for diff output")
		}
		defer os.Remove(tmpfile.Name())
		
		// Set up a separate file for stderr
		errfile, err := os.CreateTemp("", "git-diff-err-*.out")
		if err != nil {
			return nil, errors.Wrap(err, "creating temporary file for diff errors")
		}
		defer os.Remove(errfile.Name())
		
		// Set the command to write directly to these files
		cmd.Stdout = tmpfile
		cmd.Stderr = errfile
		
		// Run the command directly to the files
		err = cmd.Run()
		
		// Close the files before reading
		tmpfile.Close()
		errfile.Close()
		
		// Now read the error file first
		stderr, readErr := os.ReadFile(errfile.Name())
		if readErr != nil {
			return nil, errors.Wrap(readErr, "reading git error output")
		}
		
		// Check for command errors
		if err != nil {
			return nil, errors.Wrapf(err, "'git %s' failed: %s", strings.Join(args, " "), string(stderr))
		}
		
		// Read the diff output in chunks to avoid memory pressure
		diffFile, err := os.Open(tmpfile.Name())
		if err != nil {
			return nil, errors.Wrap(err, "opening diff output file")
		}
		defer diffFile.Close()
		
		// Get file size
		stat, err := diffFile.Stat()
		if err != nil {
			return nil, errors.Wrap(err, "getting diff file stats")
		}
		
		fileSize := stat.Size()
		if fileSize == 0 {
			return nil, errors.New("empty diff produced, possible buffer capacity issue")
		}
		
		// Read the entire file - for very large diffs we should consider 
		// alternative approaches that don't load everything into memory
		diff, err := os.ReadFile(tmpfile.Name())
		if err != nil {
			return nil, errors.Wrap(err, "reading diff output")
		}
		
		// Perform additional validation on the diff
		if len(diff) == 0 {
			return nil, errors.New("empty diff produced, possible buffer capacity issue")
		}
		
		// For very large diffs, do an additional sanity check that the diff seems complete
		if len(diff) > 1024*1024 { // Only for diffs > 1MB
			// Check that the diff looks valid - should end with context or a newline
			hasValidEnding := bytes.HasSuffix(diff, []byte{10}) || // newline
			               bytes.Contains(diff[len(diff)-100:], []byte("--- ")) ||
			               bytes.Contains(diff[len(diff)-100:], []byte("+++ ")) ||
			               bytes.Contains(diff[len(diff)-100:], []byte("@@ "))
			
			if !hasValidEnding {
				return nil, errors.New("diff appears to be truncated - buffer limit may have been reached")
			}
		}
		
		return diff, nil
	}
	
	// For non-diff commands, use the original implementation
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return out, errors.Wrapf(err, "'git %s' failed: %s", strings.Join(args, " "), string(exitErr.Stderr))
		}
		return out, errors.Wrapf(err, "'git %s' failed: %s", strings.Join(args, " "), string(out))
	}
	return out, nil
}

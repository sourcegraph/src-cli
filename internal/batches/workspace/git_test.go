package workspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRunGitCmdDisablesHooks(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	if _, err := runGitCmd(ctx, dir, "init", "--quiet"); err != nil {
		t.Fatal(err)
	}
	config, err := os.OpenFile(filepath.Join(dir, ".git", "config"), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := config.WriteString("[core]\n\thooksPath = hooks\n"); err != nil {
		t.Fatal(err)
	}
	if err := config.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "hooks"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "hooks", "pre-commit"), []byte("#!/bin/sh\necho hook-ran > hook-ran\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("test\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := runGitCmd(ctx, dir, "add", "README.md"); err != nil {
		t.Fatal(err)
	}
	if _, err := runGitCmd(ctx, dir, "commit", "--quiet", "-m", "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "hook-ran")); !os.IsNotExist(err) {
		t.Fatalf("pre-commit hook ran: %v", err)
	}
}

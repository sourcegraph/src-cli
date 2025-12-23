package lsp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUriToRepoPath(t *testing.T) {
	gitRoot, err := getGitRoot()
	require.NoError(t, err)

	tests := []struct {
		name     string
		uri      string
		wantPath string
	}{
		{
			name:     "simple file URI",
			uri:      "file://" + filepath.Join(gitRoot, "cmd/src/main.go"),
			wantPath: "cmd/src/main.go",
		},
		{
			name:     "nested path",
			uri:      "file://" + filepath.Join(gitRoot, "internal/lsp/server.go"),
			wantPath: "internal/lsp/server.go",
		},
		{
			name:     "root file",
			uri:      "file://" + filepath.Join(gitRoot, "go.mod"),
			wantPath: "go.mod",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{}
			got, err := s.uriToRepoPath(tt.uri)
			require.NoError(t, err)
			require.Equal(t, tt.wantPath, got)
		})
	}
}

func TestUriToRepoPathErrors(t *testing.T) {
	tests := []struct {
		name    string
		uri     string
		wantErr string
	}{
		{
			name:    "invalid URI",
			uri:     "://invalid",
			wantErr: "failed to parse URI",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{}
			_, err := s.uriToRepoPath(tt.uri)
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestGetGitRoot(t *testing.T) {
	root, err := getGitRoot()
	require.NoError(t, err)
	require.NotEmpty(t, root)

	info, err := os.Stat(filepath.Join(root, ".git"))
	require.NoError(t, err)
	require.True(t, info.IsDir())
}

func TestRunGitCommand(t *testing.T) {
	output, err := runGitCommand("rev-parse", "--is-inside-work-tree")
	require.NoError(t, err)
	require.Equal(t, "true", output)
}

func TestRunGitCommandError(t *testing.T) {
	_, err := runGitCommand("invalid-command-that-does-not-exist")
	require.Error(t, err)
	require.Contains(t, err.Error(), "git command failed")
}

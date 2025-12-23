package lsp

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/sourcegraph/src-cli/internal/api/mock"
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

func TestHandleTextDocumentDocumentHighlight(t *testing.T) {
	gitRoot, err := getGitRoot()
	require.NoError(t, err)

	tests := []struct {
		name      string
		path      string
		response  string
		wantCount int
		wantNil   bool
	}{
		{
			name: "filters to same file only",
			path: "main.go",
			response: `{
				"repository": {
					"commit": {
						"blob": {
							"lsif": {
								"references": {
									"nodes": [
										{
											"resource": {
												"path": "main.go",
												"repository": {"name": "github.com/test/repo"},
												"commit": {"oid": "abc123"}
											},
											"range": {
												"start": {"line": 10, "character": 0},
												"end": {"line": 10, "character": 5}
											}
										},
										{
											"resource": {
												"path": "main.go",
												"repository": {"name": "github.com/test/repo"},
												"commit": {"oid": "abc123"}
											},
											"range": {
												"start": {"line": 20, "character": 0},
												"end": {"line": 20, "character": 5}
											}
										},
										{
											"resource": {
												"path": "other.go",
												"repository": {"name": "github.com/test/repo"},
												"commit": {"oid": "abc123"}
											},
											"range": {
												"start": {"line": 5, "character": 0},
												"end": {"line": 5, "character": 5}
											}
										}
									]
								}
							}
						}
					}
				}
			}`,
			wantCount: 2,
		},
		{
			name: "filters out other repositories",
			path: "main.go",
			response: `{
				"repository": {
					"commit": {
						"blob": {
							"lsif": {
								"references": {
									"nodes": [
										{
											"resource": {
												"path": "main.go",
												"repository": {"name": "github.com/test/repo"},
												"commit": {"oid": "abc123"}
											},
											"range": {
												"start": {"line": 10, "character": 0},
												"end": {"line": 10, "character": 5}
											}
										},
										{
											"resource": {
												"path": "main.go",
												"repository": {"name": "github.com/other/repo"},
												"commit": {"oid": "def456"}
											},
											"range": {
												"start": {"line": 15, "character": 0},
												"end": {"line": 15, "character": 5}
											}
										}
									]
								}
							}
						}
					}
				}
			}`,
			wantCount: 1,
		},
		{
			name: "no references returns nil",
			path: "main.go",
			response: `{
				"repository": {
					"commit": {
						"blob": {
							"lsif": {
								"references": {
									"nodes": []
								}
							}
						}
					}
				}
			}`,
			wantNil: true,
		},
		{
			name: "all references in other files returns nil",
			path: "main.go",
			response: `{
				"repository": {
					"commit": {
						"blob": {
							"lsif": {
								"references": {
									"nodes": [
										{
											"resource": {
												"path": "other.go",
												"repository": {"name": "github.com/test/repo"},
												"commit": {"oid": "abc123"}
											},
											"range": {
												"start": {"line": 10, "character": 0},
												"end": {"line": 10, "character": 5}
											}
										}
									]
								}
							}
						}
					}
				}
			}`,
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &mock.Client{}
			mockRequest := &mock.Request{Response: tt.response}
			mockRequest.On("Do", context.Background(), &referencesResponse{}).Return(true, nil)
			mockClient.On("NewRequest", referencesQuery, map[string]any{
				"repository": "github.com/test/repo",
				"commit":     "abc123",
				"path":       tt.path,
				"line":       10,
				"character":  5,
			}).Return(mockRequest)

			s := &Server{
				apiClient: mockClient,
				repoName:  "github.com/test/repo",
				commit:    "abc123",
			}

			uri := "file://" + filepath.Join(gitRoot, tt.path)
			params := &protocol.DocumentHighlightParams{
				TextDocumentPositionParams: protocol.TextDocumentPositionParams{
					TextDocument: protocol.TextDocumentIdentifier{URI: uri},
					Position:     protocol.Position{Line: 10, Character: 5},
				},
			}

			result, err := s.handleTextDocumentDocumentHighlight(nil, params)
			require.NoError(t, err)

			if tt.wantNil {
				require.Nil(t, result)
				return
			}

			require.Len(t, result, tt.wantCount)
		})
	}
}

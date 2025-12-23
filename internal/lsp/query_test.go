package lsp

import (
	"context"
	"testing"

	"github.com/sourcegraph/src-cli/internal/api/mock"
	"github.com/stretchr/testify/require"
)

func TestQueryHover(t *testing.T) {
	tests := []struct {
		name      string
		response  string
		wantText  string
		wantRange *RangeResult
		wantNil   bool
	}{
		{
			name: "successful hover with range",
			response: `{
				"repository": {
					"commit": {
						"blob": {
							"lsif": {
								"hover": {
									"markdown": {"text": "func main()"},
									"range": {
										"start": {"line": 10, "character": 5},
										"end": {"line": 10, "character": 9}
									}
								}
							}
						}
					}
				}
			}`,
			wantText: "func main()",
			wantRange: &RangeResult{
				Start: Position{Line: 10, Character: 5},
				End:   Position{Line: 10, Character: 9},
			},
		},
		{
			name: "hover without range",
			response: `{
				"repository": {
					"commit": {
						"blob": {
							"lsif": {
								"hover": {
									"markdown": {"text": "type Config struct"}
								}
							}
						}
					}
				}
			}`,
			wantText:  "type Config struct",
			wantRange: nil,
		},
		{
			name: "no hover data",
			response: `{
				"repository": {
					"commit": {
						"blob": {
							"lsif": {
								"hover": null
							}
						}
					}
				}
			}`,
			wantNil: true,
		},
		{
			name: "no lsif data",
			response: `{
				"repository": {
					"commit": {
						"blob": {
							"lsif": null
						}
					}
				}
			}`,
			wantNil: true,
		},
		{
			name:     "empty response",
			response: `{"repository": null}`,
			wantNil:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &mock.Client{}
			mockRequest := &mock.Request{Response: tt.response}
			mockRequest.On("Do", context.Background(), &hoverResponse{}).Return(true, nil)
			mockClient.On("NewRequest", hoverQuery, map[string]any{
				"repository": "github.com/test/repo",
				"commit":     "abc123",
				"path":       "main.go",
				"line":       10,
				"character":  5,
			}).Return(mockRequest)

			s := &Server{
				apiClient: mockClient,
				repoName:  "github.com/test/repo",
				commit:    "abc123",
			}

			result, err := s.queryHover(context.Background(), "main.go", 10, 5)
			require.NoError(t, err)

			if tt.wantNil {
				require.Nil(t, result)
				return
			}

			require.NotNil(t, result)
			require.Equal(t, tt.wantText, result.Markdown.Text)
			if tt.wantRange != nil {
				require.NotNil(t, result.Range)
				require.Equal(t, tt.wantRange.Start.Line, result.Range.Start.Line)
				require.Equal(t, tt.wantRange.Start.Character, result.Range.Start.Character)
				require.Equal(t, tt.wantRange.End.Line, result.Range.End.Line)
				require.Equal(t, tt.wantRange.End.Character, result.Range.End.Character)
			} else {
				require.Nil(t, result.Range)
			}
		})
	}
}

func TestQueryDefinitions(t *testing.T) {
	tests := []struct {
		name      string
		response  string
		wantCount int
		wantNil   bool
	}{
		{
			name: "single definition",
			response: `{
				"repository": {
					"commit": {
						"blob": {
							"lsif": {
								"definitions": {
									"nodes": [{
										"resource": {
											"path": "pkg/utils.go",
											"repository": {"name": "github.com/test/repo"},
											"commit": {"oid": "abc123"}
										},
										"range": {
											"start": {"line": 20, "character": 0},
											"end": {"line": 20, "character": 10}
										}
									}]
								}
							}
						}
					}
				}
			}`,
			wantCount: 1,
		},
		{
			name: "multiple definitions",
			response: `{
				"repository": {
					"commit": {
						"blob": {
							"lsif": {
								"definitions": {
									"nodes": [
										{
											"resource": {
												"path": "pkg/a.go",
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
												"path": "pkg/b.go",
												"repository": {"name": "github.com/test/repo"},
												"commit": {"oid": "abc123"}
											},
											"range": {
												"start": {"line": 20, "character": 0},
												"end": {"line": 20, "character": 5}
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
			name: "no definitions",
			response: `{
				"repository": {
					"commit": {
						"blob": {
							"lsif": {
								"definitions": null
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
			mockRequest.On("Do", context.Background(), &definitionsResponse{}).Return(true, nil)
			mockClient.On("NewRequest", definitionsQuery, map[string]any{
				"repository": "github.com/test/repo",
				"commit":     "abc123",
				"path":       "main.go",
				"line":       10,
				"character":  5,
			}).Return(mockRequest)

			s := &Server{
				apiClient: mockClient,
				repoName:  "github.com/test/repo",
				commit:    "abc123",
			}

			result, err := s.queryDefinitions(context.Background(), "main.go", 10, 5)
			require.NoError(t, err)

			if tt.wantNil {
				require.Nil(t, result)
				return
			}

			require.Len(t, result, tt.wantCount)
		})
	}
}

func TestQueryReferences(t *testing.T) {
	tests := []struct {
		name      string
		response  string
		wantCount int
		wantNil   bool
	}{
		{
			name: "multiple references",
			response: `{
				"repository": {
					"commit": {
						"blob": {
							"lsif": {
								"references": {
									"nodes": [
										{
											"resource": {
												"path": "cmd/main.go",
												"repository": {"name": "github.com/test/repo"},
												"commit": {"oid": "abc123"}
											},
											"range": {
												"start": {"line": 15, "character": 2},
												"end": {"line": 15, "character": 8}
											}
										},
										{
											"resource": {
												"path": "pkg/handler.go",
												"repository": {"name": "github.com/test/repo"},
												"commit": {"oid": "abc123"}
											},
											"range": {
												"start": {"line": 42, "character": 10},
												"end": {"line": 42, "character": 16}
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
			name: "no references",
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
			wantCount: 0,
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
				"path":       "main.go",
				"line":       10,
				"character":  5,
			}).Return(mockRequest)

			s := &Server{
				apiClient: mockClient,
				repoName:  "github.com/test/repo",
				commit:    "abc123",
			}

			result, err := s.queryReferences(context.Background(), "main.go", 10, 5)
			require.NoError(t, err)

			if tt.wantNil {
				require.Nil(t, result)
				return
			}

			require.Len(t, result, tt.wantCount)
		})
	}
}

package lsp

import "context"

const hoverQuery = `
query Hover($repository: String!, $commit: String!, $path: String!, $line: Int!, $character: Int!) {
	repository(name: $repository) {
		commit(rev: $commit) {
			blob(path: $path) {
				lsif {
					hover(line: $line, character: $character) {
						markdown {
							text
						}
						range {
							start {
								line
								character
							}
							end {
								line
								character
							}
						}
					}
				}
			}
		}
	}
}
`

type HoverResult struct {
	Markdown struct {
		Text string `json:"text"`
	} `json:"markdown"`
	Range *RangeResult `json:"range"`
}

type RangeResult struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type hoverResponse struct {
	Repository *struct {
		Commit *struct {
			Blob *struct {
				LSIF *struct {
					Hover *HoverResult `json:"hover"`
				} `json:"lsif"`
			} `json:"blob"`
		} `json:"commit"`
	} `json:"repository"`
}

func (s *Server) queryHover(ctx context.Context, path string, line, character int) (*HoverResult, error) {
	vars := map[string]any{
		"repository": s.repoName,
		"commit":     s.commit,
		"path":       path,
		"line":       line,
		"character":  character,
	}

	var result hoverResponse
	ok, err := s.apiClient.NewRequest(hoverQuery, vars).Do(ctx, &result)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	if result.Repository == nil ||
		result.Repository.Commit == nil ||
		result.Repository.Commit.Blob == nil ||
		result.Repository.Commit.Blob.LSIF == nil ||
		result.Repository.Commit.Blob.LSIF.Hover == nil {
		return nil, nil
	}

	return result.Repository.Commit.Blob.LSIF.Hover, nil
}

const definitionsQuery = `
query Definitions($repository: String!, $commit: String!, $path: String!, $line: Int!, $character: Int!) {
	repository(name: $repository) {
		commit(rev: $commit) {
			blob(path: $path) {
				lsif {
					definitions(line: $line, character: $character) {
						nodes {
							resource {
								path
								repository {
									name
								}
								commit {
									oid
								}
							}
							range {
								start {
									line
									character
								}
								end {
									line
									character
								}
							}
						}
					}
				}
			}
		}
	}
}
`

type LocationNode struct {
	Resource struct {
		Path       string `json:"path"`
		Repository struct {
			Name string `json:"name"`
		} `json:"repository"`
		Commit struct {
			OID string `json:"oid"`
		} `json:"commit"`
	} `json:"resource"`
	Range RangeResult `json:"range"`
}

type definitionsResponse struct {
	Repository *struct {
		Commit *struct {
			Blob *struct {
				LSIF *struct {
					Definitions *struct {
						Nodes []LocationNode `json:"nodes"`
					} `json:"definitions"`
				} `json:"lsif"`
			} `json:"blob"`
		} `json:"commit"`
	} `json:"repository"`
}

func (s *Server) queryDefinitions(ctx context.Context, path string, line, character int) ([]LocationNode, error) {
	vars := map[string]any{
		"repository": s.repoName,
		"commit":     s.commit,
		"path":       path,
		"line":       line,
		"character":  character,
	}

	var result definitionsResponse
	ok, err := s.apiClient.NewRequest(definitionsQuery, vars).Do(ctx, &result)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	if result.Repository == nil ||
		result.Repository.Commit == nil ||
		result.Repository.Commit.Blob == nil ||
		result.Repository.Commit.Blob.LSIF == nil ||
		result.Repository.Commit.Blob.LSIF.Definitions == nil {
		return nil, nil
	}

	return result.Repository.Commit.Blob.LSIF.Definitions.Nodes, nil
}

const referencesQuery = `
query References($repository: String!, $commit: String!, $path: String!, $line: Int!, $character: Int!) {
	repository(name: $repository) {
		commit(rev: $commit) {
			blob(path: $path) {
				lsif {
					references(line: $line, character: $character, first: 100) {
						nodes {
							resource {
								path
								repository {
									name
								}
								commit {
									oid
								}
							}
							range {
								start {
									line
									character
								}
								end {
									line
									character
								}
							}
						}
					}
				}
			}
		}
	}
}
`

type referencesResponse struct {
	Repository *struct {
		Commit *struct {
			Blob *struct {
				LSIF *struct {
					References *struct {
						Nodes []LocationNode `json:"nodes"`
					} `json:"references"`
				} `json:"lsif"`
			} `json:"blob"`
		} `json:"commit"`
	} `json:"repository"`
}

func (s *Server) queryReferences(ctx context.Context, path string, line, character int) ([]LocationNode, error) {
	vars := map[string]any{
		"repository": s.repoName,
		"commit":     s.commit,
		"path":       path,
		"line":       line,
		"character":  character,
	}

	var result referencesResponse
	ok, err := s.apiClient.NewRequest(referencesQuery, vars).Do(ctx, &result)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	if result.Repository == nil ||
		result.Repository.Commit == nil ||
		result.Repository.Commit.Blob == nil ||
		result.Repository.Commit.Blob.LSIF == nil ||
		result.Repository.Commit.Blob.LSIF.References == nil {
		return nil, nil
	}

	return result.Repository.Commit.Blob.LSIF.References.Nodes, nil
}

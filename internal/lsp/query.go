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

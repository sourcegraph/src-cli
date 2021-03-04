package main

import (
	"bytes"
	"testing"

	"github.com/hexops/autogold"

	"github.com/sourcegraph/src-cli/internal/streaming"
)

func TestRepoTemplate(t *testing.T) {
	v, err := parseTemplate(streamingTemplate)
	if err != nil {
		t.Fatal(err)
	}

	repo := &streaming.EventRepoMatch{
		Type:       streaming.RepoMatchType,
		Repository: "sourcegraph/sourcegraph",
		Branches:   []string{},
	}

	got := new(bytes.Buffer)
	err = v.ExecuteTemplate(got, "repo", struct {
		SourcegraphEndpoint string
		*streaming.EventRepoMatch
	}{
		SourcegraphEndpoint: "https://sourcegraph.com",
		EventRepoMatch:      repo,
	})
	if err != nil {
		t.Fatal(err)
	}

	autogold.Equal(t, string(got.Bytes()))
}

func TestFileTemplate(t *testing.T) {
	v, err := parseTemplate(streamingTemplate)
	if err != nil {
		t.Fatal(err)
	}

	file := &streaming.EventFileMatch{
		Type:       streaming.FileMatchType,
		Path:       "path/to/file",
		Repository: "org/repo",
		Branches:   nil,
		Version:    "",
		LineMatches: []streaming.EventLineMatch{
			{
				Line:             "foo bar",
				LineNumber:       4,
				OffsetAndLengths: [][2]int32{{4, 3}},
			},
		},
	}

	got := new(bytes.Buffer)
	err = v.ExecuteTemplate(got, "file", struct {
		Query string
		*streaming.EventFileMatch
	}{
		Query:          "bar",
		EventFileMatch: file,
	})
	if err != nil {
		t.Fatal(err)
	}

	autogold.Equal(t, string(got.Bytes()))
}

func TestSymbolTemplate(t *testing.T) {
	v, err := parseTemplate(streamingTemplate)
	if err != nil {
		t.Fatal(err)
	}

	symbol := &streaming.EventSymbolMatch{
		Type:       streaming.FileMatchType,
		Path:       "path/to/file",
		Repository: "org/repo",
		Branches:   nil,
		Version:    "",
		Symbols: []streaming.Symbol{{
			URL:           "github.com/sourcegraph/sourcegraph/-/blob/cmd/frontend/graphqlbackend/search_results.go#L1591:26-1591:35",
			Name:          "doResults",
			ContainerName: "",
			Kind:          "FUNCTION",
		}},
	}

	got := new(bytes.Buffer)
	err = v.ExecuteTemplate(got, "symbol", struct {
		SourcegraphEndpoint string
		*streaming.EventSymbolMatch
	}{
		SourcegraphEndpoint: "https://sourcegraph.com",
		EventSymbolMatch:    symbol,
	})
	if err != nil {
		t.Fatal(err)
	}

	autogold.Equal(t, string(got.Bytes()))
}

func TestCommitTemplate(t *testing.T) {
	v, err := parseTemplate(streamingTemplate)
	if err != nil {
		t.Fatal(err)
	}

	commit := &streaming.EventCommitMatch{
		Type:    streaming.CommitMatchType,
		Icon:    "",
		Label:   "[sourcegraph/sourcegraph-atom](/github.com/sourcegraph/sourcegraph-atom) › [Stephen Gutekanst](/github.com/sourcegraph/sourcegraph-atom/-/commit/5b098d7fed963d88e23057ed99d73d3c7a33ad89): [all: release v1.0.5](/github.com/sourcegraph/sourcegraph-atom/-/commit/5b098d7fed963d88e23057ed99d73d3c7a33ad89)^",
		URL:     "",
		Detail:  "",
		Content: "```COMMIT_EDITMSG\nfoo bar\n```",
		Ranges: [][3]int32{
			{1, 3, 3},
		},
	}

	got := new(bytes.Buffer)
	err = v.ExecuteTemplate(got, "commit", struct {
		SourcegraphEndpoint string
		*streaming.EventCommitMatch
	}{
		SourcegraphEndpoint: "https://sourcegraph.com",
		EventCommitMatch:    commit,
	})
	if err != nil {
		t.Fatal(err)
	}

	autogold.Equal(t, string(got.Bytes()))
}

func TestProgressTemplate(t *testing.T) {
	v, err := parseTemplate(streamingTemplate)
	if err != nil {
		t.Fatal(err)
	}

	repoCount := 42
	progress := streaming.Progress{
		Done:              true,
		RepositoriesCount: &repoCount,
		MatchCount:        17,
		DurationMs:        421,
		Skipped:           nil,
	}

	got := new(bytes.Buffer)
	err = v.ExecuteTemplate(got, "progress", progress)
	if err != nil {
		t.Fatal(err)
	}

	autogold.Equal(t, string(got.Bytes()))
}

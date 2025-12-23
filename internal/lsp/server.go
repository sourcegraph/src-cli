package lsp

import (
	"context"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
	"github.com/tliron/glsp/server"

	"github.com/sourcegraph/sourcegraph/lib/errors"

	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/codeintel"
	"github.com/sourcegraph/src-cli/internal/version"
)

const serverName = "src-lsp"

type Server struct {
	apiClient api.Client
	repoName  string
	commit    string
}

func NewServer(apiClient api.Client) (*Server, error) {
	repoName, err := codeintel.InferRepo()
	if err != nil {
		return nil, errors.Wrap(err, "failed to infer repository name")
	}

	commit, err := codeintel.InferMergeBase()
	if err != nil {
		return nil, errors.Wrap(err, "failed to infer merge-base commit")
	}

	return &Server{
		apiClient: apiClient,
		repoName:  repoName,
		commit:    commit,
	}, nil
}

func (s *Server) Run() error {
	handler := protocol.Handler{
		Initialize:             s.handleInitialize,
		Initialized:            s.handleInitialized,
		Shutdown:               s.handleShutdown,
		SetTrace:               s.handleSetTrace,
		TextDocumentDidOpen:    s.handleTextDocumentDidOpen,
		TextDocumentDidClose:   s.handleTextDocumentDidClose,
		TextDocumentDefinition:        s.handleTextDocumentDefinition,
		TextDocumentReferences:        s.handleTextDocumentReferences,
		TextDocumentHover:             s.handleTextDocumentHover,
		TextDocumentDocumentHighlight: s.handleTextDocumentDocumentHighlight,
	}

	srv := server.NewServer(&handler, serverName, false)
	return srv.RunStdio()
}

func (s *Server) handleInitialize(_ *glsp.Context, _ *protocol.InitializeParams) (any, error) {
	return protocol.InitializeResult{
		Capabilities: protocol.ServerCapabilities{
			TextDocumentSync: &protocol.TextDocumentSyncOptions{
				OpenClose: &protocol.True,
			},
			DefinitionProvider:        true,
			ReferencesProvider:        true,
			HoverProvider:             true,
			DocumentHighlightProvider: true,
		},
		ServerInfo: &protocol.InitializeResultServerInfo{
			Name:    serverName,
			Version: &version.BuildTag,
		},
	}, nil
}

func (s *Server) handleInitialized(_ *glsp.Context, _ *protocol.InitializedParams) error {
	return nil
}

func (s *Server) handleShutdown(_ *glsp.Context) error {
	protocol.SetTraceValue(protocol.TraceValueOff)
	return nil
}

func (s *Server) handleSetTrace(_ *glsp.Context, params *protocol.SetTraceParams) error {
	protocol.SetTraceValue(params.Value)
	return nil
}

func (s *Server) handleTextDocumentDidOpen(_ *glsp.Context, _ *protocol.DidOpenTextDocumentParams) error {
	return nil
}

func (s *Server) handleTextDocumentDidClose(_ *glsp.Context, _ *protocol.DidCloseTextDocumentParams) error {
	return nil
}

func (s *Server) handleTextDocumentDefinition(_ *glsp.Context, params *protocol.DefinitionParams) (any, error) {
	path, err := s.uriToRepoPath(params.TextDocument.URI)
	if err != nil {
		return nil, err
	}

	nodes, err := s.queryDefinitions(context.Background(), path, int(params.Position.Line), int(params.Position.Character))
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, nil
	}

	gitRoot, err := getGitRoot()
	if err != nil {
		return nil, err
	}

	var locations []protocol.Location
	for _, node := range nodes {
		if node.Resource.Repository.Name != s.repoName {
			continue
		}

		absPath := filepath.Join(gitRoot, node.Resource.Path)
		uri := "file://" + absPath

		locations = append(locations, protocol.Location{
			URI: uri,
			Range: protocol.Range{
				Start: protocol.Position{
					Line:      protocol.UInteger(node.Range.Start.Line),
					Character: protocol.UInteger(node.Range.Start.Character),
				},
				End: protocol.Position{
					Line:      protocol.UInteger(node.Range.End.Line),
					Character: protocol.UInteger(node.Range.End.Character),
				},
			},
		})
	}

	if len(locations) == 0 {
		return nil, nil
	}

	return locations, nil
}

func (s *Server) handleTextDocumentReferences(_ *glsp.Context, params *protocol.ReferenceParams) ([]protocol.Location, error) {
	path, err := s.uriToRepoPath(params.TextDocument.URI)
	if err != nil {
		return nil, err
	}

	nodes, err := s.queryReferences(context.Background(), path, int(params.Position.Line), int(params.Position.Character))
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, nil
	}

	gitRoot, err := getGitRoot()
	if err != nil {
		return nil, err
	}

	var locations []protocol.Location
	for _, node := range nodes {
		if node.Resource.Repository.Name != s.repoName {
			continue
		}

		absPath := filepath.Join(gitRoot, node.Resource.Path)
		uri := "file://" + absPath

		locations = append(locations, protocol.Location{
			URI: uri,
			Range: protocol.Range{
				Start: protocol.Position{
					Line:      protocol.UInteger(node.Range.Start.Line),
					Character: protocol.UInteger(node.Range.Start.Character),
				},
				End: protocol.Position{
					Line:      protocol.UInteger(node.Range.End.Line),
					Character: protocol.UInteger(node.Range.End.Character),
				},
			},
		})
	}

	if len(locations) == 0 {
		return nil, nil
	}

	return locations, nil
}

func (s *Server) handleTextDocumentHover(_ *glsp.Context, params *protocol.HoverParams) (*protocol.Hover, error) {
	path, err := s.uriToRepoPath(params.TextDocument.URI)
	if err != nil {
		return nil, err
	}

	result, err := s.queryHover(context.Background(), path, int(params.Position.Line), int(params.Position.Character))
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	hover := &protocol.Hover{
		Contents: protocol.MarkupContent{
			Kind:  protocol.MarkupKindMarkdown,
			Value: result.Markdown.Text,
		},
	}

	if result.Range != nil {
		hover.Range = &protocol.Range{
			Start: protocol.Position{
				Line:      protocol.UInteger(result.Range.Start.Line),
				Character: protocol.UInteger(result.Range.Start.Character),
			},
			End: protocol.Position{
				Line:      protocol.UInteger(result.Range.End.Line),
				Character: protocol.UInteger(result.Range.End.Character),
			},
		}
	}

	return hover, nil
}

func (s *Server) handleTextDocumentDocumentHighlight(_ *glsp.Context, params *protocol.DocumentHighlightParams) ([]protocol.DocumentHighlight, error) {
	path, err := s.uriToRepoPath(params.TextDocument.URI)
	if err != nil {
		return nil, err
	}

	nodes, err := s.queryReferences(context.Background(), path, int(params.Position.Line), int(params.Position.Character))
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, nil
	}

	var highlights []protocol.DocumentHighlight
	for _, node := range nodes {
		if node.Resource.Repository.Name != s.repoName {
			continue
		}
		if node.Resource.Path != path {
			continue
		}

		highlights = append(highlights, protocol.DocumentHighlight{
			Range: protocol.Range{
				Start: protocol.Position{
					Line:      protocol.UInteger(node.Range.Start.Line),
					Character: protocol.UInteger(node.Range.Start.Character),
				},
				End: protocol.Position{
					Line:      protocol.UInteger(node.Range.End.Line),
					Character: protocol.UInteger(node.Range.End.Character),
				},
			},
		})
	}

	if len(highlights) == 0 {
		return nil, nil
	}

	return highlights, nil
}

func (s *Server) uriToRepoPath(uri string) (string, error) {
	parsed, err := url.Parse(uri)
	if err != nil {
		return "", errors.Wrap(err, "failed to parse URI")
	}

	absPath := parsed.Path

	gitRoot, err := getGitRoot()
	if err != nil {
		return "", errors.Wrap(err, "failed to get git root")
	}

	relPath, err := filepath.Rel(gitRoot, absPath)
	if err != nil {
		return "", errors.Wrap(err, "failed to compute relative path")
	}

	return relPath, nil
}

func getGitRoot() (string, error) {
	output, err := runGitCommand("rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return output, nil
}

func runGitCommand(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", errors.Wrapf(err, "git command failed: %s", output)
	}
	return strings.TrimSpace(string(output)), nil
}

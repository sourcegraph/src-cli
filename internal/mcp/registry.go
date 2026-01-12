package mcp

import (
	"context"
	"encoding/json"
	"iter"
	"maps"
	"slices"

	"github.com/sourcegraph/src-cli/internal/api"
)

// ToolRegistry keeps track of tools and the endpoints they originated from
type ToolRegistry struct {
	tools    map[string]*ToolDef
	endpoint string
}

func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]*ToolDef),
	}
}

// LoadTools loads the tool definitions from the MCP endpoint.
// It tries the new endpoint first (.api/mcp), then falls back to legacy (.api/mcp/v1).
func (r *ToolRegistry) LoadTools(ctx context.Context, client api.Client) error {
	endpoint, err := resolveMCPEndpoint(ctx, client)
	if err != nil {
		return err
	}
	r.endpoint = endpoint

	tools, err := fetchToolDefinitions(ctx, client, endpoint)
	if err != nil {
		return err
	}
	r.tools = tools
	return nil
}

// Get returns the tool definition for the given name
func (r *ToolRegistry) Get(name string) (*ToolDef, bool) {
	tool, ok := r.tools[name]
	return tool, ok
}

// CallTool calls the given tool with the given arguments. It constructs the Tool request and decodes the Tool response
func (r *ToolRegistry) CallTool(ctx context.Context, client api.Client, name string, args map[string]any) (map[string]json.RawMessage, error) {
	tool := r.tools[name]
	resp, err := doToolCall(ctx, client, r.endpoint, tool.RawName, args)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return decodeToolResponse(resp)
}

// All returns an iterator that yields the name and Tool definition of all registered tools
func (r *ToolRegistry) All() iter.Seq2[string, *ToolDef] {
	keys := slices.Sorted(maps.Keys(r.tools))
	return func(yield func(string, *ToolDef) bool) {
		for _, key := range keys {
			if !yield(key, r.tools[key]) {
				return
			}
		}
	}
}

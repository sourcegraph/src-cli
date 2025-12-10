package mcp

import (
	"context"
	"encoding/json"
	"iter"

	"github.com/sourcegraph/src-cli/internal/api"

	"github.com/sourcegraph/sourcegraph/lib/errors"
)

// ToolRegistry keeps track of tools and the endpoints they originated from
type ToolRegistry struct {
	tools     map[string]*ToolDef
	endpoints map[string]string
}

func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools:     make(map[string]*ToolDef),
		endpoints: make(map[string]string),
	}
}

// LoadTools loads the tool definitions from the Mcp tool endpoints constants McpURLPath and McpDeepSearchURLPath
func (r *ToolRegistry) LoadTools(ctx context.Context, client api.Client) error {
	endpoints := []string{McpURLPath, McpDeepSearchURLPath}

	var errs []error
	for _, endpoint := range endpoints {
		tools, err := fetchToolDefinitions(ctx, client, endpoint)
		if err != nil {
			errs = append(errs, errors.Wrapf(err, "failed to load tools from %s", endpoint))
			continue
		}
		r.register(endpoint, tools)
	}

	if len(errs) > 0 {
		return errors.Append(nil, errs...)
	}
	return nil
}

// register associates a collection of tools with the given endpoint
func (r *ToolRegistry) register(endpoint string, tools map[string]*ToolDef) {
	for name, def := range tools {
		r.tools[name] = def
		r.endpoints[name] = endpoint
	}
}

// Get returns the tool definition for the given name
func (r *ToolRegistry) Get(name string) (*ToolDef, bool) {
	tool, ok := r.tools[name]
	return tool, ok
}

// CallTool calls the given tool with the given arguments. It constructs the Tool request and decodes the Tool response
func (r *ToolRegistry) CallTool(ctx context.Context, client api.Client, name string, args map[string]any) (map[string]json.RawMessage, error) {
	tool := r.tools[name]
	endpoint := r.endpoints[name]
	resp, err := doToolCall(ctx, client, endpoint, tool.RawName, args)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return decodeToolResponse(resp)
}

// All returns an iterator that yields the name and Tool definition of all registered tools
func (r *ToolRegistry) All() iter.Seq2[string, *ToolDef] {
	return func(yield func(string, *ToolDef) bool) {
		for name, def := range r.tools {
			if !yield(name, def) {
				return
			}
		}
	}
}

//go:generate ../../scripts/gen-mcp-tool-json.sh mcp_tools.json
package mcp

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/sourcegraph/sourcegraph/lib/errors"
)

//go:embed mcp_tools.json
var _ []byte

type MCPToolDef struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	InputSchema  Schema `json:"inputSchema"`
	OutputSchema Schema `json:"outputSchema"`
}

type Schema struct {
	Schema string `json:"$schema"`
	SchemaObject
}

type RawSchema struct {
	Type                 string                     `json:"type"`
	Description          string                     `json:"description"`
	Schema               string                     `json:"$schema"`
	Required             []string                   `json:"required,omitempty"`
	AdditionalProperties bool                       `json:"additionalProperties"`
	Properties           map[string]json.RawMessage `json:"properties"`
	Items                json.RawMessage            `json:"items"`
}

type SchemaValue interface {
	Type() string
}

type SchemaObject struct {
	Kind                 string                 `json:"type"`
	Description          string                 `json:"description"`
	Required             []string               `json:"required,omitempty"`
	AdditionalProperties bool                   `json:"additionalProperties"`
	Properties           map[string]SchemaValue `json:"properties"`
}

func (s SchemaObject) Type() string { return s.Kind }

type SchemaArray struct {
	Kind        string      `json:"type"`
	Description string      `json:"description"`
	Items       SchemaValue `json:"items,omitempty"`
}

func (s SchemaArray) Type() string { return s.Kind }

type SchemaPrimitive struct {
	Description string `json:"description"`
	Kind        string `json:"type"`
}

func (s SchemaPrimitive) Type() string { return s.Kind }

type parser struct {
	errors []error
}

func LoadToolDefinitions(data []byte) (map[string]*MCPToolDef, error) {
	defs := struct {
		Tools []struct {
			Name         string    `json:"name"`
			Description  string    `json:"description"`
			InputSchema  RawSchema `json:"inputSchema"`
			OutputSchema RawSchema `json:"outputSchema"`
		} `json:"tools"`
	}{}

	if err := json.Unmarshal(data, &defs); err != nil {
		// TODO: think we should panic instead
		return nil, err
	}

	tools := map[string]*MCPToolDef{}
	parser := &parser{}

	for _, t := range defs.Tools {
		tools[t.Name] = &MCPToolDef{
			Name:         t.Name,
			Description:  t.Description,
			InputSchema:  parser.parseRootSchema(t.InputSchema),
			OutputSchema: parser.parseRootSchema(t.OutputSchema),
		}
	}

	if len(parser.errors) > 0 {
		return tools, errors.Append(nil, parser.errors...)
	}

	return tools, nil
}

func (p *parser) parseRootSchema(r RawSchema) Schema {
	return Schema{
		Schema: r.Schema,
		SchemaObject: SchemaObject{
			Kind:                 r.Type,
			Description:          r.Description,
			Required:             r.Required,
			AdditionalProperties: r.AdditionalProperties,
			Properties:           p.parseProperties(r.Properties),
		},
	}
}

func (p *parser) parseSchema(r *RawSchema) SchemaValue {
	switch r.Type {
	case "object":
		return &SchemaObject{
			Kind:                 r.Type,
			Description:          r.Description,
			Required:             r.Required,
			AdditionalProperties: r.AdditionalProperties,
			Properties:           p.parseProperties(r.Properties),
		}
	case "array":
		var items SchemaValue
		if len(r.Items) > 0 {
			var boolItems bool
			if err := json.Unmarshal(r.Items, &boolItems); err == nil {
				// Sometimes items is defined as "items: true", so we handle it here and
				// consider it "empty" array
			} else {
				var itemRaw RawSchema
				if err := json.Unmarshal(r.Items, &itemRaw); err == nil {
					items = p.parseSchema(&itemRaw)
				} else {
					p.errors = append(p.errors, errors.Errorf("failed to unmarshal array items: %w", err))
				}
			}
		}
		return &SchemaArray{
			Kind:        r.Type,
			Description: r.Description,
			Items:       items,
		}
	default:
		return &SchemaPrimitive{
			Kind:        r.Type,
			Description: r.Description,
		}
	}
}

func (p *parser) parseProperties(props map[string]json.RawMessage) map[string]SchemaValue {
	res := make(map[string]SchemaValue)
	for name, raw := range props {
		var r RawSchema
		if err := json.Unmarshal(raw, &r); err != nil {
			p.errors = append(p.errors, fmt.Errorf("failed to parse property %q: %w", name, err))
			continue
		}
		res[name] = p.parseSchema(&r)
	}
	return res
}

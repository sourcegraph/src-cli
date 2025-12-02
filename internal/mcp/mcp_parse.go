package mcp

import (
	_ "embed"
	"encoding/json"
	"strings"

	"github.com/sourcegraph/sourcegraph/lib/errors"
)

//go:embed mcp_tools.json
var mcpToolListJSON []byte

type ToolDef struct {
	Name         string       `json:"name"`
	Description  string       `json:"description"`
	InputSchema  SchemaObject `json:"inputSchema"`
	OutputSchema SchemaObject `json:"outputSchema"`
}

type RawSchema struct {
	Type                 string                     `json:"type"`
	Description          string                     `json:"description"`
	SchemaVersion        string                     `json:"$schema"`
	Required             []string                   `json:"required,omitempty"`
	AdditionalProperties bool                       `json:"additionalProperties"`
	Properties           map[string]json.RawMessage `json:"properties"`
	Items                json.RawMessage            `json:"items"`
}

type SchemaValue interface {
	ValueType() string
}

type SchemaObject struct {
	Type                 string                 `json:"type"`
	Description          string                 `json:"description"`
	Schema               string                 `json:"$schema,omitempty"`
	Required             []string               `json:"required,omitempty"`
	AdditionalProperties bool                   `json:"additionalProperties"`
	Properties           map[string]SchemaValue `json:"properties"`
}

func (s SchemaObject) ValueType() string { return s.Type }

type SchemaArray struct {
	Type        string      `json:"type"`
	Description string      `json:"description"`
	Items       SchemaValue `json:"items,omitempty"`
}

func (s SchemaArray) ValueType() string { return s.Type }

type SchemaPrimitive struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

func (s SchemaPrimitive) ValueType() string { return s.Type }

type decoder struct {
	errors []error
}

func LoadToolDefinitions() (map[string]*ToolDef, error) {
	return loadToolDefinitions(mcpToolListJSON)
}

func loadToolDefinitions(data []byte) (map[string]*ToolDef, error) {
	defs := struct {
		Tools []struct {
			Name         string    `json:"name"`
			Description  string    `json:"description"`
			InputSchema  RawSchema `json:"inputSchema"`
			OutputSchema RawSchema `json:"outputSchema"`
		} `json:"tools"`
	}{}

	if err := json.Unmarshal(data, &defs); err != nil {
		return nil, err
	}

	tools := map[string]*ToolDef{}
	decoder := &decoder{}

	for _, t := range defs.Tools {
		name := normalizeToolName(t.Name)
		tools[name] = &ToolDef{
			Name:         t.Name,
			Description:  t.Description,
			InputSchema:  decoder.decodeRootSchema(t.InputSchema),
			OutputSchema: decoder.decodeRootSchema(t.OutputSchema),
		}
	}

	if len(decoder.errors) > 0 {
		return tools, errors.Append(nil, decoder.errors...)
	}

	return tools, nil
}

func (d *decoder) decodeRootSchema(r RawSchema) SchemaObject {
	return SchemaObject{
		Schema:               r.SchemaVersion,
		Type:                 r.Type,
		Description:          r.Description,
		Required:             r.Required,
		AdditionalProperties: r.AdditionalProperties,
		Properties:           d.decodeProperties(r.Properties),
	}
}

func (d *decoder) decodeSchema(r *RawSchema) SchemaValue {
	switch r.Type {
	case "object":
		return &SchemaObject{
			Type:                 r.Type,
			Description:          r.Description,
			Required:             r.Required,
			AdditionalProperties: r.AdditionalProperties,
			Properties:           d.decodeProperties(r.Properties),
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
					items = d.decodeSchema(&itemRaw)
				} else {
					d.errors = append(d.errors, errors.Errorf("failed to unmarshal array items: %w", err))
				}
			}
		}
		return &SchemaArray{
			Type:        r.Type,
			Description: r.Description,
			Items:       items,
		}
	default:
		return &SchemaPrimitive{
			Type:        r.Type,
			Description: r.Description,
		}
	}
}

func (d *decoder) decodeProperties(props map[string]json.RawMessage) map[string]SchemaValue {
	res := make(map[string]SchemaValue)
	for name, raw := range props {
		var r RawSchema
		if err := json.Unmarshal(raw, &r); err != nil {
			d.errors = append(d.errors, errors.Newf("failed to parse property %q: %w", name, err))
			continue
		}
		res[name] = d.decodeSchema(&r)
	}
	return res
}

// normalizeToolName takes mcp tool names like 'sg_keyword_search' and normalizes it to 'keyword-search"
func normalizeToolName(toolName string) string {
	toolName, _ = strings.CutPrefix(toolName, "sg_")
	return strings.ReplaceAll(toolName, "_", "-")
}

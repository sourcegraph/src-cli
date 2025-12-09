package mcp

import (
	_ "embed"
	"encoding/json"
	"strings"

	"github.com/sourcegraph/sourcegraph/lib/errors"
)

type ToolDef struct {
	Name         string
	RawName      string       `json:"name"`
	Description  string       `json:"description"`
	InputSchema  SchemaObject `json:"inputSchema"`
	OutputSchema SchemaObject `json:"outputSchema"`
}

type rawSchema struct {
	Type        string                     `json:"type"`
	Description string                     `json:"description"`
	Required    []string                   `json:"required,omitempty"`
	Properties  map[string]json.RawMessage `json:"properties"`
	Items       json.RawMessage            `json:"items"`
}

type SchemaValue interface {
	ValueType() string
}

type SchemaObject struct {
	Type        string                 `json:"type"`
	Description string                 `json:"description"`
	Required    []string               `json:"required,omitempty"`
	Properties  map[string]SchemaValue `json:"properties"`

	// two fields which we do not use from the schema:
	// - $schema
	// - additionalPropterties
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

func loadToolDefinitions(data []byte) (map[string]*ToolDef, error) {
	defs := struct {
		Tools []struct {
			Name         string    `json:"name"`
			Description  string    `json:"description"`
			InputSchema  rawSchema `json:"inputSchema"`
			OutputSchema rawSchema `json:"outputSchema"`
		} `json:"tools"`
	}{}

	if err := json.Unmarshal(data, &defs); err != nil {
		return nil, err
	}

	tools := map[string]*ToolDef{}
	decoder := &decoder{}

	for _, t := range defs.Tools {
		// normalize the raw mcp tool name to be without the mcp identifiers
		rawName := t.Name
		name, _ := strings.CutPrefix(rawName, "sg_")
		name = strings.ReplaceAll(name, "_", "-")

		tool := &ToolDef{
			Name:         name,
			RawName:      rawName,
			Description:  t.Description,
			InputSchema:  decoder.decodeRootSchema(t.InputSchema),
			OutputSchema: decoder.decodeRootSchema(t.OutputSchema),
		}
		tools[tool.Name] = tool
	}

	if len(decoder.errors) > 0 {
		return tools, errors.Append(nil, decoder.errors...)
	}

	return tools, nil
}

func (d *decoder) decodeRootSchema(r rawSchema) SchemaObject {
	return SchemaObject{
		Type:        r.Type,
		Description: r.Description,
		Required:    r.Required,
		Properties:  d.decodeProperties(r.Properties),
	}
}

func (d *decoder) decodeSchema(r *rawSchema) SchemaValue {
	switch r.Type {
	case "object":
		return &SchemaObject{
			Type:        r.Type,
			Description: r.Description,
			Required:    r.Required,
			Properties:  d.decodeProperties(r.Properties),
		}
	case "array":
		var items SchemaValue
		if len(r.Items) > 0 {
			var boolItems bool
			if err := json.Unmarshal(r.Items, &boolItems); err == nil {
				// Sometimes items is defined as "items: true", so we handle it here and
				// consider it "empty" array
			} else {
				var itemRaw rawSchema
				if err := json.Unmarshal(r.Items, &itemRaw); err == nil {
					items = d.decodeSchema(&itemRaw)
				} else {
					d.errors = append(d.errors, errors.Wrap(err, "failed to unmarshal array items"))
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
		var r rawSchema
		if err := json.Unmarshal(raw, &r); err != nil {
			d.errors = append(d.errors, errors.Wrapf(err, "failed to parse property %q: %w", name))
			continue
		}
		res[name] = d.decodeSchema(&r)
	}
	return res
}

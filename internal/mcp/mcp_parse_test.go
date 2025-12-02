package mcp

import (
	"testing"
)

func TestLoadToolDefinitions(t *testing.T) {
	toolJSON := []byte(`{
	  "tools": [
		{
		  "name": "test_tool",
		  "description": "test description",
		  "inputSchema": {
			"type": "object",
			"$schema": "https://localhost/schema-draft/2025-07",
			"properties": {
			  "tags": {
				"type": "array",
				"items": {
				  "type": "object",
				  "properties": {
					"key": { "type": "string" },
					"value": { "type": "string" }
				  }
				}
			  }
			}
		  },
		  "outputSchema": {
			"type": "object",
			"$schema": "https://localhost/schema-draft/2025-07",
			"properties": {
			  "result": { "type": "string" }
			}
		  }
		}
	  ]
	}`)

	tools, err := LoadToolDefinitions(toolJSON)
	if err != nil {
		t.Fatalf("Failed to load tool definitions: %v", err)
	}

	if len(tools) != 1 {
		t.Fatalf("Expected 1 tool, got %d", len(tools))
	}

	tool := tools["test_tool"]
	if tool == nil {
		t.Fatal("Tool 'test_tool' not found")
	}

	if tool.Name != "test_tool" {
		t.Errorf("Expected name 'test_tool', got '%s'", tool.Name)
	}

	inputSchema := tool.InputSchema
	outputSchema := tool.OutputSchema
	schemaVersion := "https://localhost/schema-draft/2025-07"

	if inputSchema.Schema != schemaVersion {
		t.Errorf("Expected input schema version %q, got %q", schemaVersion, inputSchema.Schema)
	}
	if outputSchema.Schema != schemaVersion {
		t.Errorf("Expected output schema version %q, got %q", schemaVersion, outputSchema.Schema)
	}

	tagsProp, ok := inputSchema.Properties["tags"]
	if !ok {
		t.Fatal("Property 'tags' not found in inputSchema")
	}

	if tagsProp.Type() != "array" {
		t.Errorf("Expected tags type 'array', got '%s'", tagsProp.Type())
	}

	arraySchema, ok := tagsProp.(*SchemaArray)
	if !ok {
		t.Fatal("Expected SchemaArray for tags")
	}

	if arraySchema.Items == nil {
		t.Fatal("Expected items schema in array, got nil")
	}

	itemSchema := arraySchema.Items
	if itemSchema.Type() != "object" {
		t.Errorf("Expected item type 'object', got '%s'", itemSchema.Type())
	}

	objectSchema, ok := itemSchema.(*SchemaObject)
	if !ok {
		t.Fatal("Expected SchemaObject for item")
	}

	if _, ok := objectSchema.Properties["key"]; !ok {
		t.Error("Property 'key' not found in item schema")
	}
}

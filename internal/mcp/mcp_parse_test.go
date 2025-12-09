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

	tools, err := loadToolDefinitions(toolJSON)
	if err != nil {
		t.Fatalf("Failed to load tool definitions: %v", err)
	}

	if len(tools) != 1 {
		t.Fatalf("Expected 1 tool, got %d", len(tools))
	}

	tool := tools["test-tool"]
	if tool == nil {
		t.Fatal("Tool 'test_tool' not found")
	}

	if tool.RawName != "test_tool" {
		t.Errorf("Expected name 'test_tool', got '%s'", tool.RawName)
	}

	inputSchema := tool.InputSchema

	if len(tool.OutputSchema.Properties) == 0 {
		t.Fatalf("expected tool.OutputSchema.Properties not be empty")
	}

	tagsProp, ok := inputSchema.Properties["tags"]
	if !ok {
		t.Fatal("Property 'tags' not found in inputSchema")
	}

	if tagsProp.ValueType() != "array" {
		t.Errorf("Expected tags type 'array', got '%s'", tagsProp.ValueType())
	}

	arraySchema, ok := tagsProp.(*SchemaArray)
	if !ok {
		t.Fatal("Expected SchemaArray for tags")
	}

	if arraySchema.Items == nil {
		t.Fatal("Expected items schema in array, got nil")
	}

	itemSchema := arraySchema.Items
	if itemSchema.ValueType() != "object" {
		t.Errorf("Expected item type 'object', got '%s'", itemSchema.ValueType())
	}

	objectSchema, ok := itemSchema.(*SchemaObject)
	if !ok {
		t.Fatal("Expected SchemaObject for item")
	}

	if _, ok := objectSchema.Properties["key"]; !ok {
		t.Error("Property 'key' not found in item schema")
	}
}

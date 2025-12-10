package mcp

import (
	"slices"
	"testing"
)

func TestFlagSetParse(t *testing.T) {
	toolJSON := []byte(`{
	  "tools": [
		{
		  "name": "sg_test_tool",
		  "description": "test description",
		  "inputSchema": {
			"type": "object",
			"$schema": "https://localhost/schema-draft/2025-07",
			"required": ["values"],
			"properties": {
			  "repos": {
				"type": "array",
				"items": {
				  "type": "string"
				}
			  },
			  "tag": {
				"type": "string",
				"items": true
			  },
			  "count": {
				"type": "integer"
	          },
	          "boolFlag": {
				"type": "boolean"
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

	defs, err := loadToolDefinitions(toolJSON)
	if err != nil {
		t.Fatalf("failed to load tool json: %v", err)
	}

	flagSet, vars, err := BuildArgFlagSet(defs["test-tool"])
	if err != nil {
		t.Fatalf("failed to build flagset from mcp tool definition: %v", err)
	}

	if len(vars) == 0 {
		t.Fatalf("vars from buildArgFlagSet should not be empty")
	}

	args := []string{"-repos=A", "-repos=B", `-repos=["repo1", "repo2"]`, "-count=10", "-boolFlag", "-tag=testTag"}

	if err := flagSet.Parse(args); err != nil {
		t.Fatalf("flagset parsing failed: %v", err)
	}
	DerefFlagValues(flagSet, vars)

	expectedRepos := []string{"A", "B", "repo1", "repo2"}
	actualRepos, ok := vars["repos"].([]string)
	if !ok {
		t.Fatalf("failed to cast repos to []string, got %T", actualRepos)
	}
	if !slices.Equal(expectedRepos, actualRepos) {
		t.Fatalf("expected repos %v, got %v", expectedRepos, vars["repos"])
	}

	if v, ok := vars["tag"].(string); ok {
		if v != "testTag" {
			t.Fatalf("expected flag 'tag' values to have value %q but got %q", "testTag", v)
		}
	} else {
		t.Fatalf("expected flag 'tag' to have type of string but got %T", v)
	}
	if v, ok := vars["count"].(int); ok {
		if v != 10 {
			t.Fatalf("expected flag 'count' values to have value %d but got %d", 10, v)
		}
	} else {
		t.Fatalf("expected flag 'count' to have type of int but got %T", v)
	}
	if v, ok := vars["boolFlag"].(bool); ok {
		if v != true {
			t.Fatalf("expected flag 'boolFlag' values to have value %v but got %v", true, v)
		}
	} else {
		t.Fatalf("expected flag 'boolFlag' to have type of bool but got %T", v)
	}

}

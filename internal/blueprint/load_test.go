package blueprint

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name        string
		yaml        string
		wantErr     bool
		errContains string
		check       func(t *testing.T, bp *Blueprint)
	}{
		{
			name: "valid blueprint with all fields",
			yaml: `version: 1
name: test-blueprint
title: Test Blueprint
summary: A test blueprint
description: This is a test blueprint for unit testing
category: security
tags:
  - security
  - testing
batchSpecs:
  - name: fix-cve
monitors:
  - name: security-alert
insights:
  - name: vulnerability-count
    dashboards:
      - security-dashboard
dashboards:
  - name: security-dashboard
`,
			check: func(t *testing.T, bp *Blueprint) {
				if bp.Version != 1 {
					t.Errorf("Version = %d, want 1", bp.Version)
				}
				if bp.Name != "test-blueprint" {
					t.Errorf("Name = %q, want %q", bp.Name, "test-blueprint")
				}
				if bp.Title != "Test Blueprint" {
					t.Errorf("Title = %q, want %q", bp.Title, "Test Blueprint")
				}
				if len(bp.BatchSpecs) != 1 || bp.BatchSpecs[0].Name != "fix-cve" {
					t.Errorf("BatchSpecs = %v, want [{fix-cve}]", bp.BatchSpecs)
				}
				if len(bp.Monitors) != 1 || bp.Monitors[0].Name != "security-alert" {
					t.Errorf("Monitors = %v, want [{security-alert}]", bp.Monitors)
				}
				if len(bp.Insights) != 1 || bp.Insights[0].Name != "vulnerability-count" {
					t.Errorf("Insights = %v, want [{vulnerability-count [...]}]", bp.Insights)
				}
				if len(bp.Insights[0].Dashboards) != 1 || bp.Insights[0].Dashboards[0] != "security-dashboard" {
					t.Errorf("Insights[0].Dashboards = %v, want [security-dashboard]", bp.Insights[0].Dashboards)
				}
				if len(bp.Dashboards) != 1 || bp.Dashboards[0].Name != "security-dashboard" {
					t.Errorf("Dashboards = %v, want [{security-dashboard}]", bp.Dashboards)
				}
			},
		},
		{
			name: "minimal valid blueprint",
			yaml: `version: 1
name: minimal
`,
			check: func(t *testing.T, bp *Blueprint) {
				if bp.Name != "minimal" {
					t.Errorf("Name = %q, want %q", bp.Name, "minimal")
				}
			},
		},
		{
			name:        "missing name",
			yaml:        `version: 1`,
			wantErr:     true,
			errContains: "missing required field: name",
		},
		{
			name: "unsupported version",
			yaml: `version: 2
name: test
`,
			wantErr:     true,
			errContains: "unsupported blueprint version",
		},
		{
			name:        "invalid yaml",
			yaml:        `version: [invalid`,
			wantErr:     true,
			errContains: "parsing blueprint.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "blueprint.yaml"), []byte(tt.yaml), 0644); err != nil {
				t.Fatal(err)
			}

			bp, err := Load(dir)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, bp)
			}
		})
	}
}

func TestLoad_MissingFile(t *testing.T) {
	dir := t.TempDir()
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for missing blueprint.yaml")
	}
	if !contains(err.Error(), "reading blueprint.yaml") {
		t.Errorf("error %q does not mention reading blueprint.yaml", err.Error())
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

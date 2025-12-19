package blueprint

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindBlueprints(t *testing.T) {
	tmpDir := t.TempDir()

	bp1Dir := filepath.Join(tmpDir, "bp1")
	bp2Dir := filepath.Join(tmpDir, "nested", "bp2")
	emptyDir := filepath.Join(tmpDir, "empty")

	for _, dir := range []string{bp1Dir, bp2Dir, emptyDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	bp1Content := `version: 1
name: blueprint-one
title: Blueprint One
summary: First test blueprint
monitors:
  - name: mon1
`
	if err := os.WriteFile(filepath.Join(bp1Dir, "blueprint.yaml"), []byte(bp1Content), 0o644); err != nil {
		t.Fatal(err)
	}

	bp2Content := `version: 1
name: blueprint-two
title: Blueprint Two
summary: Second test blueprint
insights:
  - name: insight1
  - name: insight2
dashboards:
  - name: dash1
`
	if err := os.WriteFile(filepath.Join(bp2Dir, "blueprint.yaml"), []byte(bp2Content), 0o644); err != nil {
		t.Fatal(err)
	}

	found, err := FindBlueprints(tmpDir)
	if err != nil {
		t.Fatalf("FindBlueprints returned error: %v", err)
	}

	if len(found) != 2 {
		t.Fatalf("expected 2 blueprints, got %d", len(found))
	}

	byName := make(map[string]FoundBlueprint)
	for _, fb := range found {
		byName[fb.Blueprint.Name] = fb
	}

	fb1, ok := byName["blueprint-one"]
	if !ok {
		t.Fatal("blueprint-one not found")
	}
	if fb1.Subdir != "bp1" {
		t.Errorf("expected subdir 'bp1', got %q", fb1.Subdir)
	}
	if fb1.Blueprint.Title != "Blueprint One" {
		t.Errorf("expected title 'Blueprint One', got %q", fb1.Blueprint.Title)
	}
	if len(fb1.Blueprint.Monitors) != 1 {
		t.Errorf("expected 1 monitor, got %d", len(fb1.Blueprint.Monitors))
	}

	fb2, ok := byName["blueprint-two"]
	if !ok {
		t.Fatal("blueprint-two not found")
	}
	if fb2.Subdir != filepath.Join("nested", "bp2") {
		t.Errorf("expected subdir 'nested/bp2', got %q", fb2.Subdir)
	}
	if len(fb2.Blueprint.Insights) != 2 {
		t.Errorf("expected 2 insights, got %d", len(fb2.Blueprint.Insights))
	}
	if len(fb2.Blueprint.Dashboards) != 1 {
		t.Errorf("expected 1 dashboard, got %d", len(fb2.Blueprint.Dashboards))
	}
}

func TestFindBlueprints_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()

	found, err := FindBlueprints(tmpDir)
	if err != nil {
		t.Fatalf("FindBlueprints returned error: %v", err)
	}

	if len(found) != 0 {
		t.Errorf("expected 0 blueprints, got %d", len(found))
	}
}

func TestFindBlueprints_InvalidBlueprintSkipped(t *testing.T) {
	tmpDir := t.TempDir()

	validDir := filepath.Join(tmpDir, "valid")
	invalidDir := filepath.Join(tmpDir, "invalid")

	for _, dir := range []string{validDir, invalidDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	validContent := `version: 1
name: valid-blueprint
`
	if err := os.WriteFile(filepath.Join(validDir, "blueprint.yaml"), []byte(validContent), 0o644); err != nil {
		t.Fatal(err)
	}

	invalidContent := `version: 2
name: invalid-version
`
	if err := os.WriteFile(filepath.Join(invalidDir, "blueprint.yaml"), []byte(invalidContent), 0o644); err != nil {
		t.Fatal(err)
	}

	found, err := FindBlueprints(tmpDir)
	if err != nil {
		t.Fatalf("FindBlueprints returned error: %v", err)
	}

	if len(found) != 1 {
		t.Fatalf("expected 1 blueprint (invalid skipped), got %d", len(found))
	}

	if found[0].Blueprint.Name != "valid-blueprint" {
		t.Errorf("expected valid-blueprint, got %q", found[0].Blueprint.Name)
	}
}

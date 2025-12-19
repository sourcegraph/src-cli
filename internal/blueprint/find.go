package blueprint

import (
	"io/fs"
	"path/filepath"
)

type FoundBlueprint struct {
	Blueprint *Blueprint
	Subdir    string
}

func FindBlueprints(rootDir string) ([]FoundBlueprint, error) {
	var results []FoundBlueprint

	err := filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		if d.Name() != "blueprint.yaml" {
			return nil
		}

		blueprintDir := filepath.Dir(path)
		bp, err := Load(blueprintDir)
		if err != nil {
			return nil
		}

		subdir, err := filepath.Rel(rootDir, blueprintDir)
		if err != nil {
			subdir = blueprintDir
		}
		if subdir == "." {
			subdir = ""
		}

		results = append(results, FoundBlueprint{
			Blueprint: bp,
			Subdir:    subdir,
		})

		return nil
	})

	return results, err
}

package blueprint

import (
	"io/fs"
	"path/filepath"
)

// FindBlueprints recursively searches rootDir for blueprint.yaml files and
// returns the successfully loaded blueprints. Invalid or malformed blueprints
// are silently skipped to allow discovery to continue.
func FindBlueprints(rootDir string) ([]*Blueprint, error) {
	var results []*Blueprint

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
		bp, loadErr := Load(blueprintDir)
		if loadErr != nil {
			return nil
		}

		results = append(results, bp)

		return nil
	})

	return results, err
}

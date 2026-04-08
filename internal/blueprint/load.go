package blueprint

import (
	"os"
	"path/filepath"

	"github.com/sourcegraph/sourcegraph/lib/errors"
	"gopkg.in/yaml.v3"
)

// Load reads and validates a blueprint.yaml from blueprintDir.
func Load(blueprintDir string) (*Blueprint, error) {
	blueprintPath := filepath.Join(blueprintDir, "blueprint.yaml")

	data, err := os.ReadFile(blueprintPath)
	if err != nil {
		return nil, errors.Wrap(err, "reading blueprint.yaml")
	}

	var bp Blueprint
	if err := yaml.Unmarshal(data, &bp); err != nil {
		return nil, errors.Wrap(err, "parsing blueprint.yaml")
	}

	if err := Validate(&bp); err != nil {
		return nil, err
	}

	bp.Dir = blueprintDir
	return &bp, nil
}

// Validate checks that the blueprint has a supported version and required fields.
func Validate(bp *Blueprint) error {
	if bp.Version != 1 {
		return errors.Newf("unsupported blueprint version: %d (expected 1)", bp.Version)
	}

	if bp.Name == "" {
		return errors.New("blueprint.yaml missing required field: name")
	}

	return nil
}

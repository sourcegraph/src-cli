package telemetry

import (
	"github.com/sourcegraph/src-cli/internal/lazyregexp"

	"github.com/sourcegraph/sourcegraph/lib/errors"
)

// maxNameLength is the maximum length Sourcegraph accepts for a feature or
// action name.
const maxNameLength = 64

// featureActionRegex matches the names Sourcegraph accepts for feature and
// action: they must start with a lowercase letter and contain only letters,
// dashes, and dots (no digits, underscores, or whitespace). It mirrors the
// server-side validation in the Sourcegraph monorepo.
var featureActionRegex = lazyregexp.New(`^[a-z][a-zA-Z\-.]+$`)

// Validate reports whether feature and action satisfy Sourcegraph's naming
// rules. Events that fail validation are rejected before any request is made.
func Validate(feature, action string) error {
	if err := validateName("feature", feature); err != nil {
		return err
	}
	return validateName("action", action)
}

func validateName(kind, name string) error {
	if name == "" {
		return errors.Newf("telemetry %s must not be empty", kind)
	}
	if len(name) > maxNameLength {
		return errors.Newf("telemetry %s %q exceeds %d characters", kind, name, maxNameLength)
	}
	if !featureActionRegex.MatchString(name) {
		return errors.Newf("telemetry %s %q must match %s", kind, name, featureActionRegex.Re().String())
	}
	return nil
}

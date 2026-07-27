package main

import (
	"regexp"
	"strings"
	"testing"

	"github.com/sourcegraph/sourcegraph/lib/output"
)

// snapshotAnsiColors copies ansiColors and restores it on test cleanup,
// and also resets lib/output's force16 state so mutations in colorMode
// tests do not leak into other tests in this package or in lib/output.
func snapshotAnsiColors(t *testing.T) {
	t.Helper()
	original := make(map[string]string, len(ansiColors))
	for k, v := range ansiColors {
		original[k] = v
	}
	t.Cleanup(func() {
		for k := range ansiColors {
			delete(ansiColors, k)
		}
		for k, v := range original {
			ansiColors[k] = v
		}
		output.SetForce16Color(false)
	})
}

// TestApplyColorMode16Remap is a regression test for sourcegraph/src-cli#1144.
// When 16-color mode is enabled via config, ansiColors entries that currently
// use 256-color SGR sequences (e.g. "logo") must be remapped to plain 16-color
// SGR sequences so the terminal does not receive any 38;5;N or 48;5;N codes.
func TestApplyColorMode16Remap(t *testing.T) {
	snapshotAnsiColors(t)

	before := ansiColors["logo"]
	if before == "" {
		t.Fatal(`precondition failed: ansiColors["logo"] is empty, cannot verify remap`)
	}
	if !strings.Contains(before, "38;5;") {
		t.Fatalf(`precondition failed: ansiColors["logo"] = %q, expected a 256-color sequence containing "38;5;"`, before)
	}

	applyColorMode(true)

	after := ansiColors["logo"]
	if after == before {
		t.Fatalf(`applyColorMode(true) did not remap ansiColors["logo"]: still %q`, after)
	}
	if strings.Contains(after, "38;5;") || strings.Contains(after, "48;5;") {
		t.Fatalf(`applyColorMode(true) left a 256-color sequence in ansiColors["logo"]: %q`, after)
	}

	// Validate the remapped value is a concatenation of basic 16-color SGR
	// codes: foreground 30-37 / 90-97 and optional background 40-47 / 100-107.
	sgr16 := regexp.MustCompile(`^(\x1b\[(3[0-7]|9[0-7]|4[0-7]|10[0-7])m)+$`)
	if !sgr16.MatchString(after) {
		t.Fatalf(`ansiColors["logo"] after applyColorMode(true) = %q, want a 16-color ANSI SGR sequence`, after)
	}
}

// TestApplyColorMode16PropagatesToLibOutput verifies that force16Color also
// affects lib/output package-level Style variables (e.g. StyleLogo,
// StyleSearchMatch). Those styles are emitted by output.Output consumers such
// as batch progress bars and search rendering.
//
// The test enables 16-color mode through the config-driven helper and
// asserts that lib/output's exported styles no longer contain 38;5;N or
// 48;5;N sequences.
func TestApplyColorMode16PropagatesToLibOutput(t *testing.T) {
	snapshotAnsiColors(t)

	if before := output.StyleLogo.String(); !strings.Contains(before, "38;5;") {
		t.Fatalf(`precondition failed: output.StyleLogo = %q, expected a 256-color sequence`, before)
	}

	applyColorMode(true)

	checks := map[string]output.Style{
		"StyleLogo":        output.StyleLogo,
		"StyleWarning":     output.StyleWarning,
		"StyleSuccess":     output.StyleSuccess,
		"StyleFailure":     output.StyleFailure,
		"StyleSearchMatch": output.StyleSearchMatch,
	}
	for name, s := range checks {
		got := s.String()
		if strings.Contains(got, "38;5;") || strings.Contains(got, "48;5;") {
			t.Errorf("after applyColorMode(true), output.%s still emits 256-color SGR: %q", name, got)
		}
	}
}

// TestApplyColorMode16PreservesDisabledColors is a regression test for
// the NO_COLOR / COLOR=false / non-tty edge of sourcegraph/src-cli#1144:
// when an ansiColors entry has been disabled (set to "") the 16-color
// remap must not promote it back to an active escape. The contract is
// "force16 downgrades 256-color to 16-color", not "force16 forces some
// color on", so disabled entries must round-trip through applyColorMode(true)
// untouched.
func TestApplyColorMode16PreservesDisabledColors(t *testing.T) {
	snapshotAnsiColors(t)

	// Disable a representative mix: a custom 256-color entry, a basic
	// color, the reset code, and the already-empty alert title.
	ansiColors["logo"] = ""
	ansiColors["blue"] = ""
	ansiColors["nc"] = ""
	// "search-alert-proposed-title" is already "" in the default table;
	// listed explicitly for documentation.
	ansiColors["search-alert-proposed-title"] = ""

	applyColorMode(true)

	for _, name := range []string{
		"logo",
		"blue",
		"nc",
		"search-alert-proposed-title",
	} {
		if got := ansiColors[name]; got != "" {
			t.Errorf(`applyColorMode(true) re-enabled disabled color %q: got %q, want ""`, name, got)
		}
	}
}

// TestApplyColorMode16WithFullyDisabledTable covers the NO_COLOR /
// COLOR=false path end-to-end for sourcegraph/src-cli#1144: when the
// environment has asked us to suppress all color (every ansiColors entry
// is ""), enabling force16 must not reintroduce any escape sequence
// anywhere in the table.
func TestApplyColorMode16WithFullyDisabledTable(t *testing.T) {
	snapshotAnsiColors(t)

	// Simulate the NO_COLOR / COLOR=false branch in colors.go's init():
	// every entry blanked.
	for k := range ansiColors {
		ansiColors[k] = ""
	}

	applyColorMode(true)

	for name, code := range ansiColors {
		if code != "" {
			t.Errorf(`applyColorMode(true) reintroduced an escape for disabled color %q: got %q, want ""`, name, code)
		}
	}
}

// TestApplyColorModeFalseIsNoop ensures that disabling the 16-color override
// leaves the original ansiColors table untouched.
func TestApplyColorModeFalseIsNoop(t *testing.T) {
	snapshotAnsiColors(t)

	before := make(map[string]string, len(ansiColors))
	for k, v := range ansiColors {
		before[k] = v
	}

	applyColorMode(false)

	if len(ansiColors) != len(before) {
		t.Fatalf("applyColorMode(false) changed ansiColors size: got %d, want %d", len(ansiColors), len(before))
	}
	for k, want := range before {
		if got := ansiColors[k]; got != want {
			t.Errorf("applyColorMode(false) mutated ansiColors[%q]: got %q, want %q", k, got, want)
		}
	}
}

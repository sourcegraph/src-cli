package output

import (
	"strings"
	"testing"
)

// TestPackageStylesAvoid256ColorSGRWhenForce16Enabled is a regression
// test for sourcegraph/src-cli#1144. The issue requires src to render
// using only the terminal-theme 16-color palette WHEN the force16 mode
// is enabled (via the user's "force16Color": true config). Today every
// package-level Style variable in lib/output is defined via
// Fg256Color/Bg256Color and so emits raw 256-color SGR escapes
// (e.g. "\x1b[38;5;57m") regardless of mode, which leaks through every
// output.Output consumer regardless of the force16Color config in
// cmd/src.
//
// This test runs against lib/output's test binary, which has force16
// pinned on via setup_test.go's init(). It pins the desired end state
// under that mode: no exported package Style may emit a 256-color SGR
// introducer once force16 is in effect. (Default-mode rendering is
// not asserted here — the helpers Fg256Color/Bg256Color intentionally
// still produce 256-color escapes when force16 is off.)
func TestPackageStylesAvoid256ColorSGRWhenForce16Enabled(t *testing.T) {
	styles := map[string]Style{
		"StyleLogo":                     StyleLogo,
		"StylePending":                  StylePending,
		"StyleWarning":                  StyleWarning,
		"StyleFailure":                  StyleFailure,
		"StyleSuccess":                  StyleSuccess,
		"StyleSuggestion":               StyleSuggestion,
		"StyleSearchQuery":              StyleSearchQuery,
		"StyleSearchBorder":             StyleSearchBorder,
		"StyleSearchLink":               StyleSearchLink,
		"StyleSearchRepository":         StyleSearchRepository,
		"StyleSearchFilename":           StyleSearchFilename,
		"StyleSearchMatch":              StyleSearchMatch,
		"StyleSearchLineNumbers":        StyleSearchLineNumbers,
		"StyleSearchCommitAuthor":       StyleSearchCommitAuthor,
		"StyleSearchCommitSubject":      StyleSearchCommitSubject,
		"StyleSearchCommitDate":         StyleSearchCommitDate,
		"StyleWhiteOnPurple":            StyleWhiteOnPurple,
		"StyleGreyBackground":           StyleGreyBackground,
		"StyleSearchAlertTitle":         StyleSearchAlertTitle,
		"StyleSearchAlertDescription":   StyleSearchAlertDescription,
		"StyleSearchAlertProposedQuery": StyleSearchAlertProposedQuery,
		"StyleLinesDeleted":             StyleLinesDeleted,
		"StyleLinesAdded":               StyleLinesAdded,
		"StyleGrey":                     StyleGrey,
		"StyleYellow":                   StyleYellow,
		"StyleOrange":                   StyleOrange,
		"StyleRed":                      StyleRed,
		"StyleGreen":                    StyleGreen,
	}
	for name, s := range styles {
		got := s.String()
		if strings.Contains(got, "\x1b[38;5;") || strings.Contains(got, "\x1b[48;5;") {
			t.Errorf("%s emits a 256-color SGR sequence (want only 16-color): %q", name, got)
		}
	}
}

// TestFg256ColorAndBg256ColorAvoid256ColorSGRWhenForce16Enabled pins
// that the direct Fg256Color/Bg256Color helpers, used across the
// package for legacy 8-bit color definitions, must not emit a 256-color
// SGR introducer WHEN force16 mode is enabled (sourcegraph/src-cli#1144).
//
// This test runs against lib/output's test binary, which has force16
// pinned on via setup_test.go's init(). With force16 off, both helpers
// intentionally still emit "\x1b[38;5;Nm" / "\x1b[48;5;Nm" — they only
// downgrade to the basic 16-color palette while force16 is active.
func TestFg256ColorAndBg256ColorAvoid256ColorSGRWhenForce16Enabled(t *testing.T) {
	if got := Fg256Color(57).String(); strings.Contains(got, "\x1b[38;5;") {
		t.Errorf("Fg256Color(57) emits 256-color SGR (want only 16-color): %q", got)
	}
	if got := Bg256Color(11).String(); strings.Contains(got, "\x1b[48;5;") {
		t.Errorf("Bg256Color(11) emits 256-color SGR (want only 16-color): %q", got)
	}
}

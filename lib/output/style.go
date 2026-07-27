package output

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/grafana/regexp"
)

type Style struct{ code string }

// force16Color, when true, causes Style.String() to remap 256-color SGR
// introducers (ESC[38;5;Nm, ESC[48;5;Nm) to nearest basic 16-color SGR
// codes. Set via SetForce16Color, typically from cmd/src after reading
// the user's force16Color config. See sourcegraph/src-cli#1144.
var force16Color bool

// SetForce16Color toggles 16-color rewriting for Style.String(). While
// enabled, embedded 256-color SGR introducers are rewritten to basic
// 16-color SGR codes.
func SetForce16Color(b bool) { force16Color = b }

func (s Style) String() string {
	if force16Color {
		return Remap256To16SGR(s.code)
	}
	return s.code
}

// Line returns a FancyLine using this style as an alias for using output.Styledf(...)
func (s Style) Line(format string) FancyLine { return Styled(s, format) }

// Linef returns a FancyLine using this style as an alias for using output.Styledf(...)
func (s Style) Linef(format string, args ...any) FancyLine {
	return Styledf(s, format, args...)
}

func CombineStyles(styles ...Style) Style {
	sb := strings.Builder{}
	for _, s := range styles {
		fmt.Fprint(&sb, s)
	}
	return Style{sb.String()}
}

func Fg256Color(code int) Style { return Style{fmt.Sprintf("\033[38;5;%dm", code)} }
func Bg256Color(code int) Style { return Style{fmt.Sprintf("\033[48;5;%dm", code)} }

// sgr256Re matches a single 256-color SGR introducer: ESC[38;5;Nm (fg) or
// ESC[48;5;Nm (bg).
var sgr256Re = regexp.MustCompile(`\x1b\[(38|48);5;(\d{1,3})m`)

// Remap256To16SGR rewrites every 256-color SGR introducer in s to its
// nearest basic 16-color SGR equivalent. Other escape sequences (bold,
// reset, existing 16-color codes, etc.) pass through unchanged. Empty input
// returns empty output, which preserves NO_COLOR / COLOR=false semantics
// for callers that store color-disabled values as empty strings.
//
// See sourcegraph/src-cli#1144.
func Remap256To16SGR(s string) string {
	return sgr256Re.ReplaceAllStringFunc(s, func(match string) string {
		m := sgr256Re.FindStringSubmatch(match)
		isBackground := m[1] == "48"
		n, err := strconv.Atoi(m[2])
		if err != nil {
			return match
		}
		return fmt.Sprintf("\x1b[%dm", basic16SGR(n, isBackground))
	})
}

// basic16SGR returns a basic 16-color SGR parameter (30-37, 90-97 for
// foreground; 40-47, 100-107 for background) approximating the given 8-bit
// xterm 256-color code.
func basic16SGR(code int, isBackground bool) int {
	idx := nearest16(code)
	switch {
	case idx < 8 && !isBackground:
		return 30 + idx
	case idx < 8 && isBackground:
		return 40 + idx
	case !isBackground:
		return 90 + (idx - 8)
	default:
		return 100 + (idx - 8)
	}
}

// nearest16 collapses an xterm 8-bit color code (0..255) to an index into
// the 16-color basic palette (0..15). It is a coarse approximation; the
// goal is "no 256-color escapes leak through", not perceptual accuracy.
func nearest16(code int) int {
	switch {
	case code < 0:
		return 0
	case code <= 15:
		return code
	case code <= 231:
		// 6x6x6 RGB cube.
		n := code - 16
		r := n / 36
		g := (n / 6) % 6
		b := n % 6
		idx := 0
		if r >= 3 {
			idx |= 1
		}
		if g >= 3 {
			idx |= 2
		}
		if b >= 3 {
			idx |= 4
		}
		// Bright bit when any channel saturates the top of the cube.
		if r >= 4 || g >= 4 || b >= 4 {
			idx |= 8
		}
		return idx
	default:
		// Grayscale ramp 232..255.
		step := code - 232
		switch {
		case step < 8:
			return 0 // black
		case step < 16:
			return 8 // bright black (dark gray)
		default:
			return 15 // bright white
		}
	}
}

var (
	StyleReset      = Style{"\033[0m"}
	StyleLogo       = Fg256Color(57)
	StylePending    = Fg256Color(4)
	StyleWarning    = Fg256Color(124)
	StyleFailure    = CombineStyles(StyleBold, Fg256Color(196))
	StyleSuccess    = Fg256Color(2)
	StyleSuggestion = Fg256Color(244)

	StyleBold      = Style{"\033[1m"}
	StyleItalic    = Style{"\033[3m"}
	StyleUnderline = Style{"\033[4m"}

	// Search-specific colors.
	StyleSearchQuery         = Fg256Color(68)
	StyleSearchBorder        = Fg256Color(239)
	StyleSearchLink          = Fg256Color(237)
	StyleSearchRepository    = Fg256Color(23)
	StyleSearchFilename      = Fg256Color(69)
	StyleSearchMatch         = CombineStyles(Fg256Color(0), Bg256Color(11))
	StyleSearchLineNumbers   = Fg256Color(69)
	StyleSearchCommitAuthor  = Fg256Color(2)
	StyleSearchCommitSubject = Fg256Color(68)
	StyleSearchCommitDate    = Fg256Color(23)

	StyleWhiteOnPurple  = CombineStyles(Fg256Color(255), Bg256Color(55))
	StyleGreyBackground = CombineStyles(Fg256Color(0), Bg256Color(242))

	// Search alert specific colors.
	StyleSearchAlertTitle               = Fg256Color(124)
	StyleSearchAlertDescription         = Fg256Color(124)
	StyleSearchAlertProposedTitle       = Style{""}
	StyleSearchAlertProposedQuery       = Fg256Color(69)
	StyleSearchAlertProposedDescription = Style{""}

	StyleLinesDeleted = Fg256Color(196)
	StyleLinesAdded   = Fg256Color(2)

	// Colors
	StyleGrey   = Fg256Color(8)
	StyleYellow = Fg256Color(220)
	StyleOrange = Fg256Color(202)
	StyleRed    = Fg256Color(196)
	StyleGreen  = Fg256Color(2)
)

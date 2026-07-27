package output

// Enable force-16 rendering for lib/output's own test binary so package-
// level Style values (Fg256Color/Bg256Color-derived) and the
// Fg256Color/Bg256Color helpers render through the basic 16-color palette.
// Callers that import lib/output from another package (e.g. cmd/src) are
// not affected: this init only ships in lib/output's test binary.
//
// See sourcegraph/src-cli#1144.
func init() {
	SetForce16Color(true)
}

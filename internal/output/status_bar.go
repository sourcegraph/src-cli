package output

// StatusBar is a formatted output line with an optional emoji and style.
type StatusBar struct {
	completed bool

	label  string
	format string
	args   []interface{}
}

// Completef sets the StatusBar to completed and updates its text.
func (sb *StatusBar) Completef(format string, args ...interface{}) {
	sb.completed = true
	sb.format = format
	sb.args = args
}

// Resetf sets the status of the StatusBar to incomplete and updates its label and text.
func (sb *StatusBar) Resetf(label, format string, args ...interface{}) {
	sb.completed = false
	sb.label = label
	sb.format = format
	sb.args = args
}

// Updatef updates the StatusBar's text.
func (sb *StatusBar) Updatef(format string, args ...interface{}) {
	sb.format = format
	sb.args = args
}

func NewStatusBarWithLabel(label string) *StatusBar {
	return &StatusBar{label: label}
}

package output

// StatusBar is a formatted output line with an optional emoji and style.
type StatusBar struct {
	completed bool

	label  string
	format string
	args   []interface{}
}

func NewStatusBarWithLabel(label string) *StatusBar {
	return &StatusBar{label: label}
}

package output

import "time"

// StatusBar is a sub-element of a progress bar that displays the current status
// of a process.
type StatusBar struct {
	completed bool
	failed    bool

	label  string
	format string
	args   []any

	initialized bool
	startedAt   time.Time
	finishedAt  time.Time

	// logs holds verbose output lines to display below this status bar
	logs []string
}

// Completef sets the StatusBar to completed and updates its text.
func (sb *StatusBar) Completef(format string, args ...any) {
	sb.completed = true
	sb.format = format
	sb.args = args
	sb.finishedAt = time.Now()
}

// Failf sets the StatusBar to completed and failed and updates its text.
func (sb *StatusBar) Failf(format string, args ...any) {
	sb.Completef(format, args...)
	sb.failed = true
}

// Resetf sets the status of the StatusBar to incomplete and updates its label and text.
func (sb *StatusBar) Resetf(label, format string, args ...any) {
	sb.initialized = true
	sb.completed = false
	sb.failed = false
	sb.label = label
	sb.format = format
	sb.args = args
	sb.startedAt = time.Now()
	sb.finishedAt = time.Time{}
}

// Updatef updates the StatusBar's text.
func (sb *StatusBar) Updatef(format string, args ...any) {
	sb.initialized = true
	sb.format = format
	sb.args = args
}

func (sb *StatusBar) runtime() time.Duration {
	if sb.startedAt.IsZero() {
		return 0
	}
	if sb.finishedAt.IsZero() {
		return time.Since(sb.startedAt).Truncate(time.Second)
	}

	return sb.finishedAt.Sub(sb.startedAt).Truncate(time.Second)
}

func NewStatusBarWithLabel(label string) *StatusBar {
	return &StatusBar{label: label, startedAt: time.Now()}
}

func NewStatusBar() *StatusBar { return &StatusBar{} }

// AppendLog adds a log line to be displayed below this status bar.
func (sb *StatusBar) AppendLog(line string) {
	sb.logs = append(sb.logs, line)
}

// Logs returns the log lines for this status bar.
func (sb *StatusBar) Logs() []string {
	return sb.logs
}

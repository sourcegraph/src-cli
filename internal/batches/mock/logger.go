package mock

import (
	"bytes"
	"io"

	"github.com/sourcegraph/src-cli/internal/batches/log"
)

var _ log.TaskLogger = NoopTaskLogger{}

type NoopTaskLogger struct{}

func (tl NoopTaskLogger) Close() error                         { return nil }
func (tl NoopTaskLogger) Log(string)                           {}
func (tl NoopTaskLogger) Logf(string, ...interface{})          {}
func (tl NoopTaskLogger) MarkErrored()                         {}
func (tl NoopTaskLogger) Path() string                         { return "" }
func (tl NoopTaskLogger) PrefixWriter(prefix string) io.Writer { return &bytes.Buffer{} }

var _ log.LogManager = NoopLogManager{}

type NoopLogManager struct{}

func (lm NoopLogManager) AddTask(string) (log.TaskLogger, error) {
	return NoopTaskLogger{}, nil
}

func (lm NoopLogManager) Close() error {
	return nil
}
func (lm NoopLogManager) LogFiles() []string {
	return []string{"noop"}
}

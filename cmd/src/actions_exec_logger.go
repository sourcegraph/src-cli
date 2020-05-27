package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fatih/color"
	"github.com/mattn/go-isatty"
	"github.com/neelance/parallel"
	"github.com/pkg/errors"
	"github.com/segmentio/textio"
)

var (
	boldBlack = color.New(color.Bold, color.FgBlack)
	boldRed   = color.New(color.Bold, color.FgRed)
	boldGreen = color.New(color.Bold, color.FgGreen)
	hiGreen   = color.New(color.FgHiGreen)
	yellow    = color.New(color.FgYellow)
	grey      = color.New(color.FgHiBlack)
)

type actionLogger struct {
	verbose  bool
	keepLogs bool

	highlight func(a ...interface{}) string

	progress *progress

	mu         sync.Mutex
	logFiles   map[string]*os.File
	logWriters map[string]io.Writer
}

func newActionLogger(verbose, keepLogs bool) *actionLogger {
	useColor := isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd())
	if useColor {
		color.NoColor = false
	}
	progress := &progress{
		w: os.Stderr,
	}
	return &actionLogger{
		verbose:    verbose,
		keepLogs:   keepLogs,
		highlight:  color.New(color.Bold, color.FgGreen).SprintFunc(),
		progress:   progress,
		logFiles:   map[string]*os.File{},
		logWriters: map[string]io.Writer{},
	}
}

func (a *actionLogger) Start() {
	if a.verbose {
		fmt.Fprintln(os.Stderr)
	}
}

func (a *actionLogger) Warnf(format string, args ...interface{}) {
	if a.verbose {
		a.write("", yellow, "WARNING: "+format, args...)
	}
}

func (a *actionLogger) ActionFailed(err error, patches []PatchInput) {
	if !a.verbose {
		return
	}
	fmt.Fprintln(os.Stderr)
	if perr, ok := err.(parallel.Errors); ok {
		if len(patches) > 0 {
			yellow.Fprintf(os.Stderr, "✗  Action produced %d patches but failed with %d errors:\n\n", len(patches), len(perr))
		} else {
			yellow.Fprintf(os.Stderr, "✗  Action failed with %d errors:\n", len(perr))
		}
		for _, e := range perr {
			fmt.Fprintf(os.Stderr, "\t- %s\n", e)
		}
		fmt.Println()
	} else if err != nil {
		if len(patches) > 0 {
			yellow.Fprintf(os.Stderr, "✗  Action produced %d patches but failed with error: %s\n\n", len(patches), err)
		} else {
			yellow.Fprintf(os.Stderr, "✗  Action failed with error: %s\n\n", err)
		}
	} else {
		grey.Fprintf(os.Stderr, "✗  Action did not produce any patches.\n\n")
	}
}

func (a *actionLogger) ActionSuccess(patches []PatchInput, newLines bool) {
	if a.verbose {
		fmt.Fprintln(os.Stderr)
		format := "✔  Action produced %d patches."
		if newLines {
			format = format + "\n\n"
		}
		hiGreen.Fprintf(os.Stderr, format, len(patches))
	}
}

func (a *actionLogger) Infof(format string, args ...interface{}) {
	if a.verbose {
		a.write("", grey, format, args...)
	}
}

func (a *actionLogger) RepoCacheHit(repo ActionRepo, patchProduced bool) {
	if a.verbose {
		if patchProduced {
			a.write(repo.Name, boldGreen, "Cached result found: using cached diff.\n")
			return
		}
		a.write(repo.Name, grey, "Cached result found: no diff produced for this repository.\n")
	}
}

func (a *actionLogger) AddRepo(repo ActionRepo) (string, error) {
	prefix := "action-" + strings.Replace(strings.Replace(repo.Name, "/", "-", -1), "github.com-", "", -1)

	logFile, err := ioutil.TempFile(tempDirPrefix, prefix+"-log")
	if err != nil {
		return "", err
	}

	logWriter := io.Writer(logFile)

	a.mu.Lock()
	defer a.mu.Unlock()
	a.logFiles[repo.Name] = logFile
	a.logWriters[repo.Name] = logWriter

	return logFile.Name(), nil
}

func (a *actionLogger) RepoWriter(repoName string) (io.Writer, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	w, ok := a.logWriters[repoName]
	return w, ok
}

func (a *actionLogger) RepoStdoutStderr(repoName string) (io.Writer, io.Writer, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	w, ok := a.logWriters[repoName]

	if !a.verbose {
		return w, w, ok
	}

	stderrPrefix := fmt.Sprintf("%s -> [STDERR]: ", yellow.Sprint(repoName))
	stderr := textio.NewPrefixWriter(a.progress, stderrPrefix)

	stdoutPrefix := fmt.Sprintf("%s -> [STDOUT]: ", yellow.Sprint(repoName))
	stdout := textio.NewPrefixWriter(a.progress, stdoutPrefix)

	return io.MultiWriter(stdout, w), io.MultiWriter(stderr, w), ok
}

func (a *actionLogger) RepoFinished(repoName string, patchProduced bool, actionErr error) error {
	a.mu.Lock()
	f, ok := a.logFiles[repoName]
	if !ok {
		a.mu.Unlock()
		return nil
	}
	a.mu.Unlock()

	if actionErr != nil {
		if a.keepLogs {
			a.write(repoName, boldRed, "Action failed: %q (Logfile: %s)\n", actionErr, f.Name())
		} else {
			a.write(repoName, boldRed, "Action failed: %q\n", actionErr)
		}
	} else if patchProduced {
		a.write(repoName, boldGreen, "Finished. Patch produced.\n")
	} else {
		a.write(repoName, grey, "Finished. No patch produced.\n")
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	delete(a.logFiles, repoName)
	delete(a.logWriters, repoName)

	if !a.keepLogs && actionErr == nil {
		if err := os.Remove(f.Name()); err != nil {
			return errors.Wrap(err, "Failed to remove log file")
		}
	}

	return nil
}

func (a *actionLogger) RepoStarted(repoName, rev string, steps []*ActionStep) {
	a.write(repoName, yellow, "Starting action @ %s (%d steps)\n", rev, len(steps))
}

func (a *actionLogger) CommandStepStarted(repoName string, step int, args []string) {
	a.write(repoName, yellow, "%s command %v\n", boldBlack.Sprintf("[Step %d]", step), args)
}

func (a *actionLogger) CommandStepErrored(repoName string, step int, err error) {
	a.progress.IncStepsComplete(1)
	a.write(repoName, boldRed, "%s %s.\n", boldBlack.Sprintf("[Step %d]", step), err)
}

func (a *actionLogger) CommandStepDone(repoName string, step int) {
	a.progress.IncStepsComplete(1)
	a.write(repoName, yellow, "%s Done.\n", boldBlack.Sprintf("[Step %d]", step))
}

func (a *actionLogger) DockerStepStarted(repoName string, step int, image string) {
	a.write(repoName, yellow, "%s docker run %s\n", boldBlack.Sprintf("[Step %d]", step), image)
}

func (a *actionLogger) DockerStepErrored(repoName string, step int, err error, elapsed time.Duration) {
	a.progress.IncStepsComplete(1)
	a.write(repoName, boldRed, "%s %s. (%s)\n", boldBlack.Sprintf("[Step %d]", step), err, elapsed)
}

func (a *actionLogger) DockerStepDone(repoName string, step int, elapsed time.Duration) {
	a.progress.IncStepsComplete(1)
	a.write(repoName, yellow, "%s Done. (%s)\n", boldBlack.Sprintf("[Step %d]", step), elapsed)
}

func (a *actionLogger) write(repoName string, c *color.Color, format string, args ...interface{}) {
	if w, ok := a.RepoWriter(repoName); ok {
		fmt.Fprintf(w, format, args...)
	}

	if a.verbose {
		if len(repoName) > 0 {
			format = fmt.Sprintf("%s -> %s", c.Sprint(repoName), format)
		}
		fmt.Fprintf(a.progress, format, args...)
	}
}

type progress struct {
	jobs          int64
	stepsComplete int64
	totalSteps    int64

	mu          sync.Mutex
	w           io.Writer
	shouldClear bool
}

func (p *progress) Write(data []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	length := 50
	if p.shouldClear {
		// Clear current progress
		fmt.Fprintf(p.w, "\r")
		// TODO: We could be more precise here by taking into account the number of steps / jobs
		fmt.Fprintf(p.w, strings.Repeat(" ", 100))
		fmt.Fprintf(p.w, "\r")
	}

	if p.TotalSteps() == 0 {
		// Don't display bar until we know number of steps
		p.shouldClear = false
		return p.w.Write(data)
	}

	if !bytes.HasSuffix(data, []byte("\n")) {
		p.shouldClear = false
		return p.w.Write(data)
	}

	n, err := p.w.Write(data)
	if err != nil {
		return n, err
	}
	total := p.TotalSteps()
	done := p.StepsComplete()
	var pctDone float64
	if total > 0 {
		pctDone = float64(done) / float64(total)
	}
	bar := strings.Repeat("=", int(float64(length)*pctDone))
	if len(bar) < length {
		bar += ">"
	}
	bar += strings.Repeat(" ", length-len(bar))
	fmt.Fprintf(p.w, "[%s] steps(%d/%d) jobs(%d)", bar, p.StepsComplete(), p.TotalSteps(), p.Jobs())
	p.shouldClear = true
	return n, err
}

func (p *progress) SetTotalSteps(n int64) {
	atomic.StoreInt64(&p.totalSteps, n)
}

func (p *progress) TotalSteps() int64 {
	return atomic.LoadInt64(&p.totalSteps)
}

func (p *progress) StepsComplete() int64 {
	return atomic.LoadInt64(&p.stepsComplete)
}

func (p *progress) IncStepsComplete(delta int64) {
	atomic.AddInt64(&p.stepsComplete, delta)
}

func (p *progress) Jobs() int64 {
	return atomic.LoadInt64(&p.jobs)
}

func (p *progress) IncJobs() {
	atomic.AddInt64(&p.jobs, 1)
}

func (p *progress) DecJobs() {
	atomic.AddInt64(&p.jobs, -1)
}

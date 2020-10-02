package main

import (
	"flag"
	"sync"
	"time"

	"github.com/sourcegraph/src-cli/internal/output"
)

var (
	duration     time.Duration
	verbose      bool
	thorstenTest bool
)

func init() {
	flag.DurationVar(&duration, "progress", 8*time.Second, "time to take in the progress bar and pending samples")
	flag.BoolVar(&verbose, "verbose", false, "enable verbose mode")
	flag.BoolVar(&thorstenTest, "thorsten-test", false, "testing a new type of progress bar")
}

func main() {
	flag.Parse()

	out := output.NewOutput(flag.CommandLine.Output(), output.OutputOpts{
		Verbose: verbose,
	})

	var wg sync.WaitGroup
	progress := out.ProgressWithPendings([]output.ProgressBar{
		{Label: "Running steps", Max: 1.0},
	}, []output.FancyLine{
		output.Linef("", output.StylePending, "Starting worker..."),
		output.Linef("", output.StylePending, "Starting worker..."),
	}, nil)

	wg.Add(1)
	go func() {
		ticker := time.NewTicker(duration / 10)
		defer ticker.Stop()
		defer wg.Done()

		i := 0
		for _ = range ticker.C {
			i += 1
			if i > 10 {
				return
			}

			progress.Verbosef("%slog line %d", output.StyleWarning, i)
		}
	}()

	wg.Add(1)
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		defer wg.Done()

		start := time.Now()
		until := start.Add(duration)
		for _ = range ticker.C {
			now := time.Now()
			if now.After(until) {
				return
			}

			elapsed := time.Since(start)

			if elapsed < 1*time.Second {
				progress.StatusBarUpdatef(0, "github.com/sourcegraph/src-cli     - Downloading archive... (%s)", elapsed)
				progress.StatusBarUpdatef(1, "github.com/sourcegraph/sourcegraph - Downloading archive... (%s)", elapsed)
			} else if elapsed > 1*time.Second && elapsed < 2*time.Second {
				progress.StatusBarUpdatef(0, `github.com/sourcegraph/src-cli     - comby -in-place 'fmt.Sprintf("%%d", :[v])' 'strconv.Itoa(:[v])' main.go (%s)`, elapsed)
				progress.StatusBarUpdatef(1, `github.com/sourcegraph/sourcegraph - comby -in-place 'fmt.Sprintf("%%d", :[v])' 'strconv.Itoa(:[v])' pkg/main.go pkg/utils.go (%s)`, elapsed)
			} else if elapsed > 2*time.Second && elapsed < 4*time.Second {
				progress.StatusBarUpdatef(0, `github.com/sourcegraph/src-cli     - goimports -w main.go (%s)`, elapsed)
				if elapsed > (2*time.Second + 500*time.Millisecond) {
					progress.StatusBarUpdatef(1, `github.com/sourcegraph/sourcegraph - goimports -w pkg/main.go pkg/utils.go (%s)`, elapsed)
				}
			} else if elapsed > 4*time.Second && elapsed < 5*time.Second {
				progress.StatusBarComplete(1, output.Line(output.EmojiSuccess, output.StyleSuccess, `github.com/sourcegraph/sourcegraph - Done!`))
				if elapsed > (4*time.Second + 500*time.Millisecond) {
					progress.StatusBarComplete(0, output.Line(output.EmojiSuccess, output.StyleSuccess, `github.com/sourcegraph/src-cli     - Done!`))
				}
			} else if elapsed > 5*time.Second && elapsed < 6*time.Second {
				progress.StatusBarUpdatef(0, `github.com/sourcegraph/utils  - Downloading archive...`)
				if elapsed > (5*time.Second + 200*time.Millisecond) {
					progress.StatusBarUpdatef(1, `github.com/sourcegraph/nachos - Downloading archive...`)
				}
			} else if elapsed > 6*time.Second && elapsed < 7*time.Second {
				progress.StatusBarUpdatef(1, `github.com/sourcegraph/nachos - comby -in-place 'fmt.Sprintf("%%d", :[v])' 'strconv.Itoa(:[v])' main.go (%s)`, elapsed)
				if elapsed > (6*time.Second + 100*time.Millisecond) {
					progress.StatusBarUpdatef(0, `github.com/sourcegraph/utils  - comby -in-place 'fmt.Sprintf("%%d", :[v])' 'strconv.Itoa(:[v])' main.go (%s)`, elapsed)
				}
			} else if elapsed > 7*time.Second && elapsed < 8*time.Second {
				progress.StatusBarComplete(0, output.Line(output.EmojiSuccess, output.StyleSuccess, `github.com/sourcegraph/utils   - Done!`))
				if elapsed > (7*time.Second + 320*time.Millisecond) {
					progress.StatusBarComplete(1, output.Line(output.EmojiSuccess, output.StyleSuccess, `github.com/sourcegraph/nachos   - Done!`))
				}
			}

			progress.SetValue(0, float64(now.Sub(start))/float64(duration))
		}
	}()

	wg.Wait()

	progress.Complete()
}

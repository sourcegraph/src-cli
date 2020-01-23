package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/fatih/color"
	"github.com/gosuri/uilive"
	"github.com/sourcegraph/go-diff/diff"
)

var spinner = []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}

func NewActionUiPrinter(keepLogs bool) *actionUiPrinter {
	return &actionUiPrinter{
		lw: uilive.New(),
	}
}

type actionUiPrinter struct {
	keepLogs bool

	lw   *uilive.Writer
	lwMu sync.Mutex

	spinnerIdx int
}

func (p *actionUiPrinter) Init() {
	uilive.Out = os.Stderr
	uilive.RefreshInterval = 10 * time.Hour // TODO!(sqs): manually flush
	color.NoColor = false                   // force color even when in a pipe
}

func (p *actionUiPrinter) Start() {
	p.lw.Start()
}

func (p *actionUiPrinter) Stop() {
	p.lw.Stop()
}

func (p *actionUiPrinter) PrintStatus(repos map[ActionRepo]ActionRepoStatus) {
	p.lwMu.Lock()
	defer p.lwMu.Unlock()

	spinnerRune := spinner[p.spinnerIdx%len(spinner)]
	p.spinnerIdx++

	reposSorted := make([]ActionRepo, 0, len(repos))
	repoNameLen := 0
	for repo := range repos {
		reposSorted = append(reposSorted, repo)
		if n := utf8.RuneCountInString(repo.Name); n > repoNameLen {
			repoNameLen = n
		}
	}
	sort.Slice(reposSorted, func(i, j int) bool { return reposSorted[i].Name < reposSorted[j].Name })

	for i, repo := range reposSorted {
		status := repos[repo]

		var (
			timerDuration time.Duration

			statusColor func(string, ...interface{}) string

			statusText  string
			logFileText string
		)
		if p.keepLogs && status.LogFile != "" {
			logFileText = color.HiBlackString(status.LogFile)
		}
		switch {
		case !status.Cached && status.StartedAt.IsZero():
			statusColor = color.HiBlackString
			statusText = statusColor(string(spinnerRune))
			timerDuration = time.Since(status.EnqueuedAt)

		case !status.Cached && status.FinishedAt.IsZero():
			statusColor = color.YellowString
			statusText = statusColor(string(spinnerRune))
			timerDuration = time.Since(status.StartedAt)

		case status.Cached || !status.FinishedAt.IsZero():
			if status.Err != nil {
				statusColor = color.RedString
				statusText = "error: see " + status.LogFile
				logFileText = "" // don't show twice
			} else {
				statusColor = color.GreenString
				if status.Patch != (CampaignPlanPatch{}) && status.Patch.Patch != "" {
					fileDiffs, err := diff.ParseMultiFileDiff([]byte(status.Patch.Patch))
					if err != nil {
						panic(err)
						// return errors.Wrapf(err, "invalid patch for repository %q", repo.Name)
					}
					statusText = diffStatDescription(fileDiffs) + " " + diffStatDiagram(sumDiffStats(fileDiffs))
					if status.Cached {
						statusText += " (cached)"
					}
				} else {
					statusText = color.HiBlackString("0 files changed")
				}
			}
			timerDuration = status.FinishedAt.Sub(status.StartedAt)
		}

		var w io.Writer
		if i == 0 {
			w = p.lw
		} else {
			w = p.lw.Newline()
		}

		var appendTexts []string
		if statusText != "" {
			appendTexts = append(appendTexts, statusText)
		}
		if logFileText != "" {
			appendTexts = append(appendTexts, logFileText)
		}
		repoText := statusColor(fmt.Sprintf("%-*s", repoNameLen, repo.Name))
		pipe := color.HiBlackString("|")
		fmt.Fprintf(w, "%s %s ", repoText, pipe)
		fmt.Fprintf(w, "%s", strings.Join(appendTexts, " "))
		if timerDuration != 0 {
			fmt.Fprintf(w, color.HiBlackString(" %s"), timerDuration.Round(time.Second))
		}
		fmt.Fprintln(w)
	}

	_ = p.lw.Flush()
}

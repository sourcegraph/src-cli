package main

import (
	"fmt"
	"time"

	"github.com/sourcegraph/go-diff/diff"
	"github.com/sourcegraph/src-cli/internal/campaigns"
	"github.com/sourcegraph/src-cli/internal/output"
)

func newCampaignProgressPrinter(out *output.Output, numParallelism int) *campaignProgressPrinter {
	return &campaignProgressPrinter{
		out: out,

		numParallelism: numParallelism,

		completedTasks: map[string]bool{},
		runningTasks:   map[string]*campaigns.TaskStatus{},

		repoStatusBar: map[string]int{},
		statusBarRepo: map[int]string{},
	}
}

type campaignProgressPrinter struct {
	out      *output.Output
	progress output.ProgressWithStatusBars

	maxRepoName    int
	numParallelism int

	completedTasks map[string]bool
	runningTasks   map[string]*campaigns.TaskStatus

	repoStatusBar map[string]int
	statusBarRepo map[int]string
}

func (p *campaignProgressPrinter) initProgressBar(statuses []*campaigns.TaskStatus) {
	statusBars := []*output.StatusBar{}
	for i := 0; i < p.numParallelism; i++ {
		statusBars = append(statusBars, output.NewStatusBarWithLabel("Starting worker..."))
	}

	p.progress = p.out.ProgressWithStatusBars([]output.ProgressBar{{
		Label: fmt.Sprintf("Executing steps in %d repositories", len(statuses)),
		Max:   float64(len(statuses)),
	}}, statusBars, nil)
}

func (p *campaignProgressPrinter) Complete() {
	if p.progress != nil {
		p.progress.Complete()
	}
}

func (p *campaignProgressPrinter) PrintStatuses(statuses []*campaigns.TaskStatus) {
	if p.progress == nil {
		p.initProgressBar(statuses)
	}

	unloggedCompleted := []*campaigns.TaskStatus{}
	currentlyRunning := []*campaigns.TaskStatus{}

	for _, ts := range statuses {
		if len(ts.RepoName) > p.maxRepoName {
			p.maxRepoName = len(ts.RepoName)
		}

		if !ts.Running && !ts.FinishedAt.IsZero() {
			if !p.completedTasks[ts.RepoName] {
				p.completedTasks[ts.RepoName] = true
				unloggedCompleted = append(unloggedCompleted, ts)
			}
			if _, ok := p.runningTasks[ts.RepoName]; ok {
				delete(p.runningTasks, ts.RepoName)

				// Free slot
				idx := p.repoStatusBar[ts.RepoName]
				delete(p.statusBarRepo, idx)
			}
		} else if ts.Running {
			currentlyRunning = append(currentlyRunning, ts)
		}

	}

	p.progress.SetValue(0, float64(len(p.completedTasks)))

	started := map[string]*campaigns.TaskStatus{}
	runningIndex := 0
	for _, ts := range currentlyRunning {
		if _, ok := p.runningTasks[ts.RepoName]; !ok {
			started[ts.RepoName] = ts
			p.runningTasks[ts.RepoName] = ts

			// Find free slot
			_, ok := p.statusBarRepo[runningIndex]
			for ok {
				runningIndex += 1
				_, ok = p.statusBarRepo[runningIndex]
			}

			p.statusBarRepo[runningIndex] = ts.RepoName
			p.repoStatusBar[ts.RepoName] = runningIndex
		}
	}

	for _, ts := range unloggedCompleted {
		var statusText string

		if ts.ChangesetSpec == nil {
			statusText = "No changes"
		} else {
			fileDiffs, err := diff.ParseMultiFileDiff([]byte(ts.ChangesetSpec.Commits[0].Diff))
			if err != nil {
				panic(err)
			}

			statusText = diffStatDescription(fileDiffs) + " " + diffStatDiagram(sumDiffStats(fileDiffs))
		}

		if ts.Cached {
			statusText += " (cached)"
		}

		p.progress.Verbosef("%-*s %s", p.maxRepoName, ts.RepoName, statusText)

		if idx, ok := p.repoStatusBar[ts.RepoName]; ok {
			// Log that this task completed, but only if there is no
			// currently executing one in this bar, to avoid flicker.
			if _, ok := p.statusBarRepo[idx]; !ok {
				p.progress.StatusBarCompletef(idx, "Done in %s", time.Since(ts.StartedAt).Truncate(time.Millisecond))
			}
			delete(p.repoStatusBar, ts.RepoName)
		}
	}

	for statusBar, repo := range p.statusBarRepo {
		if ts, ok := started[repo]; ok {
			p.progress.StatusBarResetf(statusBar, repo, "%s (%s)", ts.CurrentlyExecuting, time.Since(ts.StartedAt).Truncate(time.Millisecond))
			continue
		}

		ts := p.runningTasks[repo]
		p.progress.StatusBarUpdatef(statusBar, "%s (%s)", ts.CurrentlyExecuting, time.Since(ts.StartedAt).Truncate(time.Millisecond))
	}
}

package main

import (
	"flag"
	"os"
	"time"

	"github.com/sourcegraph/sourcegraph/lib/output"
)

func main() {
	verbose := flag.Bool("v", false, "Enable verbose logging")
	flag.Parse()

	out := output.NewOutput(os.Stdout, output.OutputOpts{Verbose: *verbose})

	progressBar := output.ProgressBar{
		Label: "Overall progress",
		Max:   100,
		Value: 0,
	}

	statusBar1 := output.NewStatusBarWithLabel("Task 1")
	statusBar2 := output.NewStatusBarWithLabel("Task 2")
	statusBar3 := output.NewStatusBarWithLabel("Task 3")

	progress := out.ProgressWithStatusBars(
		[]output.ProgressBar{progressBar},
		[]*output.StatusBar{statusBar1, statusBar2, statusBar3},
		nil,
	)

	type step struct {
		bar     int
		name    string
		message string
	}

	steps := []step{
		{0, "archive_download", "Downloading archive..."},
		{0, "archive_extract", "Extracting files..."},
		{0, "data_process", "Processing data..."},
		{1, "workspace_init", "Initializing workspace..."},
		{1, "deps_install", "Installing dependencies..."},
		{1, "build", "Running build..."},
		{2, "tests", "Running tests..."},
		{2, "report", "Generating report..."},
		{2, "cleanup", "Cleaning up..."},
	}

	for i, s := range steps {
		progress.StatusBarUpdatef(s.bar, s.message)
		progress.StatusBarVerboseLogf(s.bar, "Step %d started: %s", i+1, s.name)

		time.Sleep(500 * time.Millisecond)

		progress.SetValue(0, float64((i+1)*100/len(steps)))
		progress.StatusBarVerboseLogf(s.bar, "Step %d finished successfully", i+1)
	}

	progress.StatusBarCompletef(0, "Done!")
	progress.StatusBarVerboseLogf(0, "All steps completed successfully")
	progress.StatusBarCompletef(1, "Done!")
	progress.StatusBarVerboseLogf(1, "All steps completed successfully")
	progress.StatusBarCompletef(2, "Done!")
	progress.StatusBarVerboseLogf(2, "All steps completed successfully")

	time.Sleep(500 * time.Millisecond)
	progress.Complete()
}

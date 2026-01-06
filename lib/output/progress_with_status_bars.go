package output

type ProgressWithStatusBars interface {
	Progress

	StatusBarUpdatef(i int, format string, args ...any)
	StatusBarCompletef(i int, format string, args ...any)
	StatusBarFailf(i int, format string, args ...any)
	StatusBarResetf(i int, label, format string, args ...any)

	// WriteBelow writes text below the status bars (appears after the UI)
	WriteBelow(s string)
	WriteBelowf(format string, args ...any)
	VerboseBelow(s string)
	VerboseBelowf(format string, args ...any)

	// StatusBarLogf appends a log line below a specific status bar
	StatusBarLogf(i int, format string, args ...any)
	// StatusBarVerboseLogf appends a log line below a specific status bar (only in verbose mode)
	StatusBarVerboseLogf(i int, format string, args ...any)
}

func newProgressWithStatusBars(bars []ProgressBar, statusBars []*StatusBar, o *Output, opts *ProgressOpts) ProgressWithStatusBars {
	barPtrs := make([]*ProgressBar, len(bars))
	for i := range bars {
		barPtrs[i] = &bars[i]
	}

	if !o.caps.Isatty {
		return newProgressWithStatusBarsSimple(barPtrs, statusBars, o, opts)
	}

	return newProgressWithStatusBarsTTY(barPtrs, statusBars, o, opts)
}

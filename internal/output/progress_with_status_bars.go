package output

type ProgressWithStatusBars interface {
	Progress

	StatusBarUpdatef(i int, format string, args ...interface{})
	StatusBarComplete(i int, message FancyLine)
}

func newProgressWithStatusBars(bars []ProgressBar, statusBars []FancyLine, o *Output, opts *ProgressOpts) ProgressWithStatusBars {
	barPtrs := make([]*ProgressBar, len(bars))
	for i := range bars {
		barPtrs[i] = &bars[i]
	}

	statusBarPtrs := make([]*FancyLine, len(statusBars))
	for i := range statusBars {
		statusBarPtrs[i] = &statusBars[i]
	}

	if !o.caps.Isatty {
		panic("not supported lol")
	}

	return newProgressWithStatusBarsTTY(barPtrs, statusBarPtrs, o, opts)
}

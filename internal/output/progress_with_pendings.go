package output

type ProgressWithPendings interface {
	Progress

	Updatef(i int, format string, args ...interface{})
	CompletePending(i int, message FancyLine)
	DestroyPending(i int)
}

func newProgressWithPendings(bars []ProgressBar, lines []FancyLine, o *Output, opts *ProgressOpts) ProgressWithPendings {
	barPtrs := make([]*ProgressBar, len(bars))
	for i := range bars {
		barPtrs[i] = &bars[i]
	}

	linePtrs := make([]*FancyLine, len(lines))
	for i := range lines {
		linePtrs[i] = &lines[i]
	}

	if !o.caps.Isatty {
		panic("not supported lol")
	}

	return newProgressWithPendingsTTY(barPtrs, linePtrs, o, opts)
}

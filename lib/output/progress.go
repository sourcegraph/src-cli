package output

type Progress interface {
	Context

	// Complete stops the set of progress bars and marks them all as completed.
	Complete()

	// Destroy stops the set of progress bars and clears them from the
	// terminal.
	Destroy()

	// SetLabel updates the label for the given bar.
	SetLabel(i int, label string)

	// SetLabelAndRecalc updates the label for the given bar and recalculates
	// the maximum width of the labels.
	SetLabelAndRecalc(i int, label string)

	// SetValue updates the value for the given bar.
	SetValue(i int, v float64)
}

type ProgressBar struct {
	Label string
	Max   float64
	Value float64

	labelWidth int
}

type ProgressOpts struct {
	PendingStyle Style
	SuccessEmoji string
	SuccessStyle Style

	// NoSpinner disables the background goroutine that updates progress bars
	// and spinners. In TTY mode this stops the spinner animation; in non-TTY
	// (simple) mode this suppresses the periodic ticker that dumps progress
	// bars, which is useful when output cannot be overwritten in place (e.g.
	// CI log files). Completion and failure events still print.
	NoSpinner bool
}

func (opt *ProgressOpts) WithNoSpinner(noSpinner bool) *ProgressOpts {
	c := *opt
	c.NoSpinner = noSpinner
	return &c
}

func newProgress(bars []ProgressBar, o *Output, opts *ProgressOpts) Progress {
	barPtrs := make([]*ProgressBar, len(bars))
	for i := range bars {
		barPtrs[i] = &bars[i]
	}

	if !o.caps.Isatty {
		return newProgressSimple(barPtrs, o, opts)
	}

	return newProgressTTY(barPtrs, o, opts)
}

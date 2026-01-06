package output

import (
	"fmt"
	"time"

	"github.com/mattn/go-runewidth"
)

func newProgressWithStatusBarsTTY(bars []*ProgressBar, statusBars []*StatusBar, o *Output, opts *ProgressOpts) *progressWithStatusBarsTTY {
	p := &progressWithStatusBarsTTY{
		progressTTY: &progressTTY{
			bars:         bars,
			o:            o,
			emojiWidth:   3,
			pendingEmoji: spinnerStrings[0],
			spinner:      newSpinner(100 * time.Millisecond),
		},
		statusBars: statusBars,
	}

	if opts != nil {
		p.opts = *opts
	} else {
		p.opts = *DefaultProgressTTYOpts
	}

	if w := runewidth.StringWidth(p.opts.SuccessEmoji); w > p.emojiWidth {
		p.emojiWidth = w + 1
	}

	p.labelWidth = calcLabelWidth(p.bars, p.emojiWidth, p.o.caps.Width)
	p.statusBarLabelWidth = calcStatusBarLabelWidth(p.statusBars, p.o.caps.Width)

	p.o.Lock()
	defer p.o.Unlock()

	p.draw()

	if opts != nil && opts.NoSpinner {
		return p
	}

	go func() {
		for s := range p.spinner.C {
			func() {
				p.pendingEmoji = s

				p.o.Lock()
				defer p.o.Unlock()

				p.moveToOrigin()
				p.draw()
			}()
		}
	}()

	return p
}

type progressWithStatusBarsTTY struct {
	*progressTTY

	statusBars           []*StatusBar
	statusBarLabelWidth  int
	numPrintedStatusBars int
	numPrintedLogLines   int
	linesBelowCount      int
	completed            bool
}

func (p *progressWithStatusBarsTTY) Close() { p.Destroy() }

func (p *progressWithStatusBarsTTY) Destroy() {
	p.spinner.stop()

	p.o.Lock()
	defer p.o.Unlock()

	p.moveToOrigin()

	for range p.lines() + p.linesBelowCount {
		p.o.clearCurrentLine()
		p.o.moveDown(1)
	}

	p.moveToOrigin()
}

func (p *progressWithStatusBarsTTY) Complete() {
	p.spinner.stop()

	p.o.Lock()
	defer p.o.Unlock()

	// Clear any lines written below the status bars
	for range p.linesBelowCount {
		p.o.clearCurrentLine()
		p.o.moveUp(1)
	}

	// +1 because of the line between progress and status bars
	for range p.numPrintedStatusBars + p.numPrintedLogLines + 1 {
		p.o.moveUp(1)
		p.o.clearCurrentLine()
	}

	for _, bar := range p.bars {
		bar.Value = bar.Max
	}

	p.completed = true
	p.o.moveUp(len(p.bars))
	p.draw()

	// Move cursor below the progress bar UI and the "below" lines
	p.o.moveDown(p.linesBelowCount)
}

func (p *progressWithStatusBarsTTY) lines() int {
	return len(p.bars) + p.numPrintedStatusBars + p.numPrintedLogLines + 1
}

func (p *progressWithStatusBarsTTY) SetLabel(i int, label string) {
	p.o.Lock()
	defer p.o.Unlock()

	p.bars[i].Label = label
	p.bars[i].labelWidth = runewidth.StringWidth(label)
	p.drawInSitu()
}

func (p *progressWithStatusBarsTTY) SetValue(i int, v float64) {
	p.o.Lock()
	defer p.o.Unlock()

	p.bars[i].Value = v
	p.drawInSitu()
}

func (p *progressWithStatusBarsTTY) StatusBarResetf(i int, label, format string, args ...any) {
	p.o.Lock()
	defer p.o.Unlock()

	if p.statusBars[i] != nil {
		p.statusBars[i].Resetf(label, format, args...)
	}

	p.statusBarLabelWidth = calcStatusBarLabelWidth(p.statusBars, p.o.caps.Width)
	p.drawInSitu()
}

func (p *progressWithStatusBarsTTY) StatusBarUpdatef(i int, format string, args ...any) {
	p.o.Lock()
	defer p.o.Unlock()

	if p.statusBars[i] != nil {
		p.statusBars[i].Updatef(format, args...)
	}

	p.drawInSitu()
}

func (p *progressWithStatusBarsTTY) StatusBarCompletef(i int, format string, args ...any) {
	p.o.Lock()
	defer p.o.Unlock()

	if p.statusBars[i] != nil {
		p.statusBars[i].Completef(format, args...)
	}

	p.drawInSitu()
}

func (p *progressWithStatusBarsTTY) StatusBarFailf(i int, format string, args ...any) {
	p.o.Lock()
	defer p.o.Unlock()

	if p.statusBars[i] != nil {
		p.statusBars[i].Failf(format, args...)
	}

	p.drawInSitu()
}

func (p *progressWithStatusBarsTTY) draw() {
	for _, bar := range p.bars {
		p.writeBar(bar)
	}

	if len(p.statusBars) > 0 {
		lineStyle := StylePending
		if p.completed {
			lineStyle = StyleSuccess
		}
		p.o.clearCurrentLine()
		fmt.Fprint(p.o.w, lineStyle, "│", runewidth.FillLeft("\n", p.o.caps.Width-1))
	}

	p.numPrintedStatusBars = 0
	p.numPrintedLogLines = 0
	for i, statusBar := range p.statusBars {
		if statusBar == nil {
			continue
		}
		if !statusBar.initialized {
			continue
		}

		last := i == len(p.statusBars)-1
		p.writeStatusBar(last, statusBar)
		p.numPrintedStatusBars += 1

		for _, logLine := range statusBar.Logs() {
			p.writeStatusBarLog(logLine, last)
			p.numPrintedLogLines++
		}
	}
}

func (p *progressWithStatusBarsTTY) moveToOrigin() {
	p.o.moveUp(p.lines())
}

func (p *progressWithStatusBarsTTY) drawInSitu() {
	p.moveToOrigin()
	p.draw()
}

func calcStatusBarLabelWidth(statusBars []*StatusBar, termWidth int) int {
	statusBarLabelWidth := 0
	for _, bar := range statusBars {
		labelWidth := runewidth.StringWidth(bar.label)
		if labelWidth > statusBarLabelWidth {
			statusBarLabelWidth = labelWidth
		}
	}

	statusBarPrefixWidth := 4 // statusBars have box char and space
	if maxWidth := termWidth/2 - statusBarPrefixWidth; (statusBarLabelWidth + 2) > maxWidth {
		statusBarLabelWidth = maxWidth - 2
	}

	return statusBarLabelWidth

}

func (p *progressWithStatusBarsTTY) writeStatusBar(last bool, statusBar *StatusBar) {
	style := StylePending
	if statusBar.completed {
		if statusBar.failed {
			style = StyleWarning
		} else {
			style = StyleSuccess
		}
	}

	box := "├── "
	if last {
		box = "└── "
	}
	const boxWidth = 4

	labelFillWidth := p.statusBarLabelWidth + 2
	label := runewidth.FillRight(runewidth.Truncate(statusBar.label, p.statusBarLabelWidth, "..."), labelFillWidth)

	duration := statusBar.runtime().String()
	durationLength := runewidth.StringWidth(duration)

	textMaxLength := p.o.caps.Width - boxWidth - labelFillWidth - (durationLength + 2)
	text := runewidth.Truncate(fmt.Sprintf(statusBar.format, p.o.caps.formatArgs(statusBar.args)...), textMaxLength, "...")

	// The text might contain invisible control characters, so we need to
	// exclude them when counting length
	textLength := visibleStringWidth(text)

	durationMaxWidth := textMaxLength - textLength + (durationLength + 2)
	durationText := runewidth.FillLeft(duration, durationMaxWidth)

	p.o.clearCurrentLine()
	fmt.Fprint(p.o.w, style, box, label, StyleReset, text, StyleBold, durationText, StyleReset, "\n")
}

func (p *progressWithStatusBarsTTY) writeStatusBarLog(logLine string, last bool) {
	indent := "│       "
	if last {
		indent = "        "
	}
	lineStyle := StylePending
	if p.completed {
		lineStyle = StyleSuccess
	}
	p.o.clearCurrentLine()
	fmt.Fprint(p.o.w, lineStyle, indent, StyleReset, logLine, "\n")
}

func (p *progressWithStatusBarsTTY) Verbose(s string) {
	if p.o.verbose {
		p.Write(s)
	}
}

func (p *progressWithStatusBarsTTY) Verbosef(format string, args ...any) {
	if p.o.verbose {
		p.Writef(format, args...)
	}
}

func (p *progressWithStatusBarsTTY) VerboseLine(line FancyLine) {
	if p.o.verbose {
		p.WriteLine(line)
	}
}

func (p *progressWithStatusBarsTTY) Write(s string) {
	p.o.Lock()
	defer p.o.Unlock()

	p.moveToOrigin()
	p.o.clearCurrentLine()
	fmt.Fprintln(p.o.w, s)
	p.draw()
}

func (p *progressWithStatusBarsTTY) Writef(format string, args ...any) {
	p.o.Lock()
	defer p.o.Unlock()

	p.moveToOrigin()
	p.o.clearCurrentLine()
	fmt.Fprintf(p.o.w, format, p.o.caps.formatArgs(args)...)
	fmt.Fprint(p.o.w, "\n")
	p.draw()
}

func (p *progressWithStatusBarsTTY) WriteLine(line FancyLine) {
	p.o.Lock()
	defer p.o.Unlock()

	p.moveToOrigin()
	p.o.clearCurrentLine()
	line.write(p.o.w, p.o.caps)
	p.draw()
}

func (p *progressWithStatusBarsTTY) WriteBelow(s string) {
	p.o.Lock()
	defer p.o.Unlock()

	p.o.moveDown(p.linesBelowCount)
	p.o.clearCurrentLine()
	fmt.Fprintln(p.o.w, s)
	p.linesBelowCount++
	p.o.moveUp(p.lines() + p.linesBelowCount)
	p.draw()
}

func (p *progressWithStatusBarsTTY) WriteBelowf(format string, args ...any) {
	p.o.Lock()
	defer p.o.Unlock()

	p.o.moveDown(p.linesBelowCount)
	p.o.clearCurrentLine()
	fmt.Fprintf(p.o.w, format, p.o.caps.formatArgs(args)...)
	fmt.Fprint(p.o.w, "\n")
	p.linesBelowCount++
	p.o.moveUp(p.lines() + p.linesBelowCount)
	p.draw()
}

func (p *progressWithStatusBarsTTY) VerboseBelow(s string) {
	if p.o.verbose {
		p.WriteBelow(s)
	}
}

func (p *progressWithStatusBarsTTY) VerboseBelowf(format string, args ...any) {
	if p.o.verbose {
		p.WriteBelowf(format, args...)
	}
}

func (p *progressWithStatusBarsTTY) StatusBarLogf(i int, format string, args ...any) {
	p.o.Lock()
	defer p.o.Unlock()

	if p.statusBars[i] != nil {
		logLine := fmt.Sprintf(format, p.o.caps.formatArgs(args)...)
		p.statusBars[i].AppendLog(logLine)
	}

	p.drawInSitu()
}

func (p *progressWithStatusBarsTTY) StatusBarVerboseLogf(i int, format string, args ...any) {
	if p.o.verbose {
		p.StatusBarLogf(i, format, args...)
	}
}

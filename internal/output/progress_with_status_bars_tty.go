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
		p.opts = defaultProgressTTYOpts
	}

	p.determineEmojiWidth()
	p.determineLabelWidth()
	p.determineStatusBarLabelWidth()

	p.o.lock.Lock()
	defer p.o.lock.Unlock()

	p.draw()

	go func() {
		for s := range p.spinner.C {
			func() {
				p.pendingEmoji = s

				p.o.lock.Lock()
				defer p.o.lock.Unlock()

				p.moveToOrigin()
				p.draw()
			}()
		}
	}()

	return p
}

type progressWithStatusBarsTTY struct {
	*progressTTY

	statusBars          []*StatusBar
	statusBarLabelWidth int
}

func (p *progressWithStatusBarsTTY) Close() { p.Destroy() }

func (p *progressWithStatusBarsTTY) Destroy() {
	p.spinner.stop()

	p.o.lock.Lock()
	defer p.o.lock.Unlock()

	p.moveToOrigin()

	for i := 0; i < p.lines(); i += 1 {
		p.o.clearCurrentLine()
		p.o.moveDown(1)
	}

	p.moveToOrigin()
}

func (p *progressWithStatusBarsTTY) Complete() {
	p.spinner.stop()

	p.o.lock.Lock()
	defer p.o.lock.Unlock()

	// +1 because of the line between progress and status bars
	for i := 0; i < len(p.statusBars)+1; i += 1 {
		p.o.moveUp(1)
		p.o.clearCurrentLine()
	}
	p.statusBars = p.statusBars[0:0]

	for _, bar := range p.bars {
		bar.Value = bar.Max
	}

	p.o.moveUp(len(p.bars))
	p.draw()
}

func (p *progressWithStatusBarsTTY) lines() int {
	return len(p.bars) + len(p.statusBars) + 1
}

func (p *progressWithStatusBarsTTY) SetLabel(i int, label string) {
	p.o.lock.Lock()
	defer p.o.lock.Unlock()

	p.bars[i].Label = label
	p.bars[i].labelWidth = runewidth.StringWidth(label)
	p.drawInSitu()
}

func (p *progressWithStatusBarsTTY) SetValue(i int, v float64) {
	p.o.lock.Lock()
	defer p.o.lock.Unlock()

	p.bars[i].Value = v
	p.drawInSitu()
}

func (p *progressWithStatusBarsTTY) StatusBarResetf(i int, label, format string, args ...interface{}) {
	p.o.lock.Lock()
	defer p.o.lock.Unlock()

	if p.statusBars[i] != nil {
		p.statusBars[i].Resetf(label, format, args...)
	}

	p.determineStatusBarLabelWidth()
	p.drawInSitu()
}

func (p *progressWithStatusBarsTTY) StatusBarUpdatef(i int, format string, args ...interface{}) {
	p.o.lock.Lock()
	defer p.o.lock.Unlock()

	if p.statusBars[i] != nil {
		p.statusBars[i].Updatef(format, args...)
	}

	p.drawInSitu()
}

func (p *progressWithStatusBarsTTY) StatusBarCompletef(i int, format string, args ...interface{}) {
	p.o.lock.Lock()
	defer p.o.lock.Unlock()

	if p.statusBars[i] != nil {
		p.statusBars[i].Completef(format, args...)
	}

	p.drawInSitu()
}

func (p *progressWithStatusBarsTTY) draw() {
	for _, bar := range p.bars {
		p.writeBar(bar)
	}

	if len(p.statusBars) > 0 {
		fmt.Fprint(p.o.w, StylePending, "┃\n")
	}

	for i, statusBar := range p.statusBars {
		if statusBar == nil {
			continue
		}
		last := i == len(p.statusBars)-1
		p.writeStatusBar(last, statusBar)
	}
}

func (p *progressWithStatusBarsTTY) moveToOrigin() {
	p.o.moveUp(p.lines())
}

func (p *progressWithStatusBarsTTY) drawInSitu() {
	p.moveToOrigin()
	p.draw()
}

func (p *progressWithStatusBarsTTY) determineStatusBarLabelWidth() {
	p.statusBarLabelWidth = 0
	for _, bar := range p.statusBars {
		labelWidth := runewidth.StringWidth(bar.label)
		if labelWidth > p.statusBarLabelWidth {
			p.statusBarLabelWidth = labelWidth
		}
	}

	statusBarPrefixWidth := 4 // statusBars have box char and space
	if maxWidth := p.o.caps.Width/2 - statusBarPrefixWidth; (p.statusBarLabelWidth + 2) > maxWidth {
		p.statusBarLabelWidth = maxWidth - 2
	}
}

func (p *progressWithStatusBarsTTY) writeStatusBar(last bool, statusBar *StatusBar) {
	style := StylePending
	if statusBar.completed {
		style = StyleSuccess
	}
	box := "┣━━ "
	if last {
		box = "┗━━ "
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

func (p *progressWithStatusBarsTTY) Verbose(s string) {
	if p.o.opts.Verbose {
		p.Write(s)
	}
}

func (p *progressWithStatusBarsTTY) Verbosef(format string, args ...interface{}) {
	if p.o.opts.Verbose {
		p.Writef(format, args...)
	}
}

func (p *progressWithStatusBarsTTY) VerboseLine(line FancyLine) {
	if p.o.opts.Verbose {
		p.WriteLine(line)
	}
}

func (p *progressWithStatusBarsTTY) Write(s string) {
	p.o.lock.Lock()
	defer p.o.lock.Unlock()

	p.moveToOrigin()
	p.o.clearCurrentLine()
	fmt.Fprintln(p.o.w, s)
	p.draw()
}

func (p *progressWithStatusBarsTTY) Writef(format string, args ...interface{}) {
	p.o.lock.Lock()
	defer p.o.lock.Unlock()

	p.moveToOrigin()
	p.o.clearCurrentLine()
	fmt.Fprintf(p.o.w, format, p.o.caps.formatArgs(args)...)
	fmt.Fprint(p.o.w, "\n")
	p.draw()
}

func (p *progressWithStatusBarsTTY) WriteLine(line FancyLine) {
	p.o.lock.Lock()
	defer p.o.lock.Unlock()

	p.moveToOrigin()
	p.o.clearCurrentLine()
	line.write(p.o.w, p.o.caps)
	p.draw()
}

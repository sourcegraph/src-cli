package output

import (
	"bytes"
	"fmt"
	"time"

	"github.com/mattn/go-runewidth"
)

func newProgressWithStatusBarsTTY(bars []*ProgressBar, statusBars []*StatusBar, o *Output, opts *ProgressOpts) *progressWithStatusBarsTTY {
	p := &progressWithStatusBarsTTY{
		bars:       bars,
		statusBars: statusBars,

		o:            o,
		emojiWidth:   3,
		pendingEmoji: spinnerStrings[0],
		spinner:      newSpinner(100 * time.Millisecond),
	}

	if opts != nil {
		p.opts = *opts
	} else {
		p.opts = ProgressOpts{
			SuccessEmoji: "\u2705",
			SuccessStyle: StyleSuccess,
			PendingStyle: StylePending,
		}
	}

	if w := runewidth.StringWidth(p.opts.SuccessEmoji); w > p.emojiWidth {
		p.emojiWidth = w + 1
	}

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
	bars []*ProgressBar

	statusBars          []*StatusBar
	statusBarLabelWidth int

	o    *Output
	opts ProgressOpts

	emojiWidth   int
	labelWidth   int
	pendingEmoji string
	spinner      *spinner
}

func (p *progressWithStatusBarsTTY) Complete() {
	p.spinner.stop()

	p.o.lock.Lock()
	defer p.o.lock.Unlock()

	for _, bar := range p.bars {
		bar.Value = bar.Max
	}

	// TODO: This makes the statusBars disappear when completing the
	// progressbar but it's a bit janky
	// p.o.moveUp(len(p.statusBars) + len(p.bars))
	// p.statusBars = p.statusBars[0:0]

	p.drawInSitu()
}

func (p *progressWithStatusBarsTTY) Close() { p.Destroy() }

func (p *progressWithStatusBarsTTY) Destroy() {
	p.spinner.stop()

	p.o.lock.Lock()
	defer p.o.lock.Unlock()

	p.moveToOrigin()
	for i := 0; i < len(p.bars); i += 1 {
		p.o.clearCurrentLine()
		p.o.moveDown(1)
	}

	p.moveToOrigin()
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

func (p *progressWithStatusBarsTTY) StatusBarResetf(i int, label, format string, args ...interface{}) {
	p.o.lock.Lock()
	defer p.o.lock.Unlock()

	if p.statusBars[i] != nil {
		p.statusBars[i].completed = false
		p.statusBars[i].label = label
		p.statusBars[i].format = format
		p.statusBars[i].args = args
	}
	p.determineStatusBarLabelWidth()

	p.drawInSitu()
}

func (p *progressWithStatusBarsTTY) StatusBarUpdatef(i int, format string, args ...interface{}) {
	p.o.lock.Lock()
	defer p.o.lock.Unlock()

	if p.statusBars[i] != nil {
		p.statusBars[i].format = format
		p.statusBars[i].args = args
	}

	p.drawInSitu()
}

func (p *progressWithStatusBarsTTY) StatusBarComplete(i int, format string, args ...interface{}) {
	p.o.lock.Lock()
	defer p.o.lock.Unlock()

	if p.statusBars[i] != nil {
		p.statusBars[i].completed = true
		p.statusBars[i].format = format
		p.statusBars[i].args = args
	}

	p.drawInSitu()
}

func (p *progressWithStatusBarsTTY) draw() {
	for i, statusBar := range p.statusBars {
		if statusBar == nil {
			continue
		}
		p.writeStatusBar(i, statusBar)
	}

	for _, bar := range p.bars {
		p.writeBar(bar)
	}
}

func (p *progressWithStatusBarsTTY) drawInSitu() {
	p.moveToOrigin()
	p.draw()
}

func (p *progressWithStatusBarsTTY) moveToOrigin() {
	p.o.moveUp(len(p.statusBars) + len(p.bars))
}

func (p *progressWithStatusBarsTTY) writeBar(bar *ProgressBar) {
	writeProgressBar(p.o, bar, p.opts, p.emojiWidth, p.labelWidth, p.pendingEmoji)
}

func (p *progressWithStatusBarsTTY) determineLabelWidth() {
	p.labelWidth = 0
	for _, bar := range p.bars {
		bar.labelWidth = runewidth.StringWidth(bar.Label)
		if bar.labelWidth > p.labelWidth {
			p.labelWidth = bar.labelWidth
		}
	}

	if maxWidth := p.o.caps.Width/2 - p.emojiWidth; (p.labelWidth + 2) > maxWidth {
		p.labelWidth = maxWidth - 2
	}

}

func (p *progressWithStatusBarsTTY) determineStatusBarLabelWidth() {
	p.statusBarLabelWidth = 0
	for _, bar := range p.statusBars {
		labelWidth := runewidth.StringWidth(bar.label)
		if labelWidth > p.statusBarLabelWidth {
			p.statusBarLabelWidth = labelWidth
		}
	}

	if maxWidth := p.o.caps.Width/2 - p.emojiWidth; (p.statusBarLabelWidth + 2) > maxWidth {
		p.statusBarLabelWidth = maxWidth - 2
	}
}

func (p *progressWithStatusBarsTTY) writeStatusBar(i int, statusBar *StatusBar) {
	p.o.clearCurrentLine()

	var out bytes.Buffer

	emoji := p.pendingEmoji
	style := StylePending
	if statusBar.completed {
		emoji = EmojiSuccess
		style = StyleSuccess
	}
	box := "┣ "
	if i == 0 {
		box = "┏ "
	}

	fmt.Fprintf(&out, "%s", p.o.caps.formatArgs([]interface{}{style})...)

	fmt.Fprint(&out, box+emoji+" ")
	fmt.Fprint(&out, runewidth.FillRight(runewidth.Truncate(statusBar.label, p.statusBarLabelWidth, "..."), p.statusBarLabelWidth))
	fmt.Fprint(&out, "  ")

	fmt.Fprintf(&out, statusBar.format+"%s", p.o.caps.formatArgs(append(statusBar.args, StyleReset))...)
	(&out).Write([]byte("\n"))

	fmt.Fprint(p.o.w, runewidth.Truncate(out.String(), p.o.caps.Width, "...\n"))
}

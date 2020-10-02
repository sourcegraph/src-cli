package output

import (
	"bytes"
	"fmt"
	"time"

	"github.com/mattn/go-runewidth"
)

type progressWithStatusBarsTTY struct {
	bars       []*ProgressBar
	statusBars []*FancyLine

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

func (p *progressWithStatusBarsTTY) StatusBarUpdatef(i int, format string, args ...interface{}) {
	p.o.lock.Lock()
	defer p.o.lock.Unlock()

	if p.statusBars[i] != nil {
		p.statusBars[i].style = StylePending
		p.statusBars[i].emoji = p.pendingEmoji
		p.statusBars[i].format = format
		p.statusBars[i].args = args
	}

	p.drawInSitu()
}

func (p *progressWithStatusBarsTTY) StatusBarComplete(i int, message FancyLine) {
	p.o.lock.Lock()
	defer p.o.lock.Unlock()

	p.statusBars[i] = &message

	p.drawInSitu()
}

func newProgressWithStatusBarsTTY(bars []*ProgressBar, statusBars []*FancyLine, o *Output, opts *ProgressOpts) *progressWithStatusBarsTTY {
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

	p.labelWidth = 0
	for _, bar := range bars {
		bar.labelWidth = runewidth.StringWidth(bar.Label)
		if bar.labelWidth > p.labelWidth {
			p.labelWidth = bar.labelWidth
		}
	}

	if maxWidth := p.o.caps.Width/2 - p.emojiWidth; (p.labelWidth + 2) > maxWidth {
		p.labelWidth = maxWidth - 2
	}

	p.o.lock.Lock()
	defer p.o.lock.Unlock()

	p.draw()

	go func() {
		for s := range p.spinner.C {
			func() {
				p.pendingEmoji = s

				p.o.lock.Lock()
				defer p.o.lock.Unlock()

				for _, line := range p.statusBars {
					if line.emoji != EmojiSuccess {
						line.emoji = s
					}
				}

				p.moveToOrigin()
				p.draw()
			}()
		}
	}()

	return p
}

func (p *progressWithStatusBarsTTY) draw() {
	for _, statusBar := range p.statusBars {
		if statusBar == nil {
			continue
		}
		p.writeStatusBar(statusBar)
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

func (p *progressWithStatusBarsTTY) writeStatusBar(statusBar *FancyLine) {
	p.o.clearCurrentLine()

	var out bytes.Buffer
	if statusBar.emoji != "" {
		fmt.Fprint(&out, statusBar.emoji+" ")
	}
	fmt.Fprintf(&out, "%s"+statusBar.format+"%s", p.o.caps.formatArgs(append(append([]interface{}{statusBar.style}, statusBar.args...), StyleReset))...)
	(&out).Write([]byte("\n"))

	fmt.Fprint(p.o.w, runewidth.Truncate(out.String(), p.o.caps.Width, "...\n"))
}

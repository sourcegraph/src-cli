package output

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/mattn/go-runewidth"
)

var DefaultProgressTTYOpts = &ProgressOpts{
	SuccessEmoji: "\u2705",
	SuccessStyle: StyleSuccess,
	PendingStyle: StylePending,
}

type progressTTY struct {
	bars []*ProgressBar

	o    *Output
	opts ProgressOpts

	emojiWidth   int
	labelWidth   int
	pendingEmoji string
	spinner      *spinner
}

func (p *progressTTY) Complete() {
	p.spinner.stop()

	p.o.Lock()
	defer p.o.Unlock()

	for _, bar := range p.bars {
		bar.Value = bar.Max
	}
	p.drawInSitu()
}

func (p *progressTTY) Close() { p.Destroy() }

func (p *progressTTY) Destroy() {
	p.spinner.stop()

	p.o.Lock()
	defer p.o.Unlock()

	p.moveToOrigin()
	for i := 0; i < len(p.bars); i += 1 {
		p.o.clearCurrentLine()
		p.o.moveDown(1)
	}

	p.moveToOrigin()
}

func (p *progressTTY) SetLabel(i int, label string) {
	p.o.Lock()
	defer p.o.Unlock()

	p.bars[i].Label = label
	p.bars[i].labelWidth = runewidth.StringWidth(label)
	p.drawInSitu()
}

func (p *progressTTY) SetLabelAndRecalc(i int, label string) {
	p.o.Lock()
	defer p.o.Unlock()

	p.bars[i].Label = label
	p.bars[i].labelWidth = runewidth.StringWidth(label)

	p.determineLabelWidth()
	p.drawInSitu()
}

func (p *progressTTY) SetValue(i int, v float64) {
	p.o.Lock()
	defer p.o.Unlock()

	p.bars[i].Value = v
	p.drawInSitu()
}

func (p *progressTTY) Verbose(s string) {
	if p.o.opts.Verbose {
		p.Write(s)
	}
}

func (p *progressTTY) Verbosef(format string, args ...interface{}) {
	if p.o.opts.Verbose {
		p.Writef(format, args...)
	}
}

func (p *progressTTY) VerboseLine(line FancyLine) {
	if p.o.opts.Verbose {
		p.WriteLine(line)
	}
}

func (p *progressTTY) Write(s string) {
	p.o.Lock()
	defer p.o.Unlock()

	p.moveToOrigin()
	p.o.clearCurrentLine()
	fmt.Fprintln(p.o.w, s)
	p.draw()
}

func (p *progressTTY) Writef(format string, args ...interface{}) {
	p.o.Lock()
	defer p.o.Unlock()

	p.moveToOrigin()
	p.o.clearCurrentLine()
	fmt.Fprintf(p.o.w, format, p.o.caps.formatArgs(args)...)
	fmt.Fprint(p.o.w, "\n")
	p.draw()
}

func (p *progressTTY) WriteLine(line FancyLine) {
	p.o.Lock()
	defer p.o.Unlock()

	p.moveToOrigin()
	p.o.clearCurrentLine()
	line.write(p.o.w, p.o.caps)
	p.draw()
}

func newProgressTTY(bars []*ProgressBar, o *Output, opts *ProgressOpts) *progressTTY {
	p := &progressTTY{
		bars:         bars,
		o:            o,
		emojiWidth:   3,
		pendingEmoji: spinnerStrings[0],
		spinner:      newSpinner(100 * time.Millisecond),
	}

	if opts != nil {
		p.opts = *opts
	} else {
		p.opts = *DefaultProgressTTYOpts
	}

	p.determineEmojiWidth()
	p.determineLabelWidth()

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

func (p *progressTTY) determineEmojiWidth() {
	if w := runewidth.StringWidth(p.opts.SuccessEmoji); w > p.emojiWidth {
		p.emojiWidth = w + 1
	}
}

func (p *progressTTY) determineLabelWidth() {
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

func (p *progressTTY) draw() {
	for _, bar := range p.bars {
		p.writeBar(bar)
	}
}

func (p *progressTTY) drawInSitu() {
	p.moveToOrigin()
	p.draw()
}

func (p *progressTTY) moveToOrigin() {
	p.o.moveUp(len(p.bars))
}

func (p *progressTTY) writeBar(bar *ProgressBar) {
	p.o.clearCurrentLine()

	value := bar.Value
	if bar.Value >= bar.Max {
		p.o.writeStyle(p.opts.SuccessStyle)
		fmt.Fprint(p.o.w, runewidth.FillRight(p.opts.SuccessEmoji, p.emojiWidth))
		value = bar.Max
	} else {
		p.o.writeStyle(p.opts.PendingStyle)
		fmt.Fprint(p.o.w, runewidth.FillRight(p.pendingEmoji, p.emojiWidth))
	}

	fmt.Fprint(p.o.w, runewidth.FillRight(runewidth.Truncate(bar.Label, p.labelWidth, "..."), p.labelWidth))

	// The bar width is the width of the terminal, minus the label width, minus
	// two spaces.
	barWidth := p.o.caps.Width - p.labelWidth - p.emojiWidth - 2

	// Unicode box drawing gives us eight possible bar widths, so we need to
	// calculate both the bar width and then the final character, if any.
	var segments int
	if bar.Max > 0 {
		segments = int(math.Round((float64(8*barWidth) * value) / bar.Max))
	}

	fillWidth := segments / 8
	remainder := segments % 8
	if remainder == 0 {
		if fillWidth > barWidth {
			fillWidth = barWidth
		}
	} else {
		if fillWidth+1 > barWidth {
			fillWidth = barWidth - 1
		}
	}

	fmt.Fprintf(p.o.w, "  ")
	fmt.Fprint(p.o.w, strings.Repeat("█", fillWidth))
	fmt.Fprintln(p.o.w, []string{
		"",
		"▏",
		"▎",
		"▍",
		"▌",
		"▋",
		"▊",
		"▉",
	}[remainder])

	p.o.writeStyle(StyleReset)
}

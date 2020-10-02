package output

import (
	"bytes"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/mattn/go-runewidth"
)

type progressWithPendingsTTY struct {
	bars  []*ProgressBar
	lines []*FancyLine

	o    *Output
	opts ProgressOpts

	emojiWidth   int
	labelWidth   int
	pendingEmoji string
	spinner      *spinner
}

func (p *progressWithPendingsTTY) Complete() {
	p.spinner.stop()

	p.o.lock.Lock()
	defer p.o.lock.Unlock()

	for _, bar := range p.bars {
		bar.Value = bar.Max
	}
	p.drawInSitu()
}

func (p *progressWithPendingsTTY) Close() { p.Destroy() }

func (p *progressWithPendingsTTY) Destroy() {
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

func (p *progressWithPendingsTTY) SetLabel(i int, label string) {
	p.o.lock.Lock()
	defer p.o.lock.Unlock()

	p.bars[i].Label = label
	p.bars[i].labelWidth = runewidth.StringWidth(label)
	p.drawInSitu()
}

func (p *progressWithPendingsTTY) SetValue(i int, v float64) {
	p.o.lock.Lock()
	defer p.o.lock.Unlock()

	p.bars[i].Value = v
	p.drawInSitu()
}

func (p *progressWithPendingsTTY) Verbose(s string) {
	if p.o.opts.Verbose {
		p.Write(s)
	}
}

func (p *progressWithPendingsTTY) Verbosef(format string, args ...interface{}) {
	if p.o.opts.Verbose {
		p.Writef(format, args...)
	}
}

func (p *progressWithPendingsTTY) VerboseLine(line FancyLine) {
	if p.o.opts.Verbose {
		p.WriteLine(line)
	}
}

func (p *progressWithPendingsTTY) Write(s string) {
	p.o.lock.Lock()
	defer p.o.lock.Unlock()

	p.moveToOrigin()
	p.o.clearCurrentLine()
	fmt.Fprintln(p.o.w, s)
	p.draw()
}

func (p *progressWithPendingsTTY) Writef(format string, args ...interface{}) {
	p.o.lock.Lock()
	defer p.o.lock.Unlock()

	p.moveToOrigin()
	p.o.clearCurrentLine()
	fmt.Fprintf(p.o.w, format, p.o.caps.formatArgs(args)...)
	fmt.Fprint(p.o.w, "\n")
	p.draw()
}

func (p *progressWithPendingsTTY) WriteLine(line FancyLine) {
	p.o.lock.Lock()
	defer p.o.lock.Unlock()

	p.moveToOrigin()
	p.o.clearCurrentLine()
	line.write(p.o.w, p.o.caps)
	p.draw()
}

func (p *progressWithPendingsTTY) Updatef(i int, format string, args ...interface{}) {
	p.o.lock.Lock()
	defer p.o.lock.Unlock()

	if p.lines[i] != nil {
		p.lines[i].style = StylePending
		p.lines[i].emoji = p.pendingEmoji
		p.lines[i].format = format
		p.lines[i].args = args
	}

	p.drawInSitu()
	// p.o.moveUp(1)
	// p.o.clearCurrentLine()
	// p.write(p.line)
}

func (p *progressWithPendingsTTY) CompletePending(i int, message FancyLine) {
	// p.spinner.stop()

	p.o.lock.Lock()
	defer p.o.lock.Unlock()

	p.lines[i] = &message

	p.drawInSitu()
}

func (p *progressWithPendingsTTY) DestroyPending(i int) {
	// p.spinner.stop()

	p.o.lock.Lock()
	defer p.o.lock.Unlock()

	p.lines[i] = nil

	p.drawInSitu()
}

func newProgressWithPendingsTTY(bars []*ProgressBar, lines []*FancyLine, o *Output, opts *ProgressOpts) *progressWithPendingsTTY {
	p := &progressWithPendingsTTY{
		bars:  bars,
		lines: lines,

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

				for _, line := range p.lines {
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

func (p *progressWithPendingsTTY) draw() {
	for _, line := range p.lines {
		if line == nil {
			continue
		}

		p.o.clearCurrentLine()

		var buf bytes.Buffer

		line.write(&buf, p.o.caps)

		// Straight up copied from (*pendingTTY).write, see comment/warnings there
		fmt.Fprint(p.o.w, runewidth.Truncate(buf.String(), p.o.caps.Width, "...\n"))
	}

	for _, bar := range p.bars {
		p.writeBar(bar)
	}
}

func (p *progressWithPendingsTTY) drawInSitu() {
	p.moveToOrigin()
	p.draw()
}

func (p *progressWithPendingsTTY) moveToOrigin() {
	p.o.moveUp(len(p.lines) + len(p.bars))
}

func (p *progressWithPendingsTTY) writeBar(bar *ProgressBar) {
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

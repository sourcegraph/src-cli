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
	*progressTTY

	statusBars          []*StatusBar
	statusBarLabelWidth int
}

func (p *progressWithStatusBarsTTY) Destroy() {
	p.spinner.stop()

	p.o.lock.Lock()
	defer p.o.lock.Unlock()

	p.moveToOrigin()

	for i := 0; i < len(p.bars)+len(p.statusBars); i += 1 {
		p.o.clearCurrentLine()
		p.o.moveDown(1)
	}

	p.moveToOrigin()
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

func (p *progressWithStatusBarsTTY) moveToOrigin() {
	p.o.moveUp(len(p.statusBars) + len(p.bars))
}

func (p *progressWithStatusBarsTTY) determineStatusBarLabelWidth() {
	p.statusBarLabelWidth = 0
	for _, bar := range p.statusBars {
		labelWidth := runewidth.StringWidth(bar.label)
		if labelWidth > p.statusBarLabelWidth {
			p.statusBarLabelWidth = labelWidth
		}
	}

	statusBarEmojiWidth := p.emojiWidth + 1 // statusBars have a space more at
	// the beginning
	if maxWidth := p.o.caps.Width/2 - statusBarEmojiWidth; (p.statusBarLabelWidth + 2) > maxWidth {
		p.statusBarLabelWidth = maxWidth - 2
	}
}

func (p *progressWithStatusBarsTTY) writeStatusBar(i int, statusBar *StatusBar) {
	emoji := p.pendingEmoji
	style := StylePending
	if statusBar.completed {
		emoji = EmojiSuccess
		style = StyleSuccess
	}

	labelFillWidth := p.statusBarLabelWidth + 2
	label := runewidth.FillRight(runewidth.Truncate(statusBar.label, p.statusBarLabelWidth, "..."), labelFillWidth)

	textMaxLength := p.o.caps.Width - (p.emojiWidth + 1) - labelFillWidth
	text := runewidth.Truncate(fmt.Sprintf(statusBar.format, p.o.caps.formatArgs(statusBar.args)...), textMaxLength, "...")

	p.o.clearCurrentLine()
	fmt.Fprint(p.o.w, style, " ", emoji, " ", label, text, StyleReset, "\n")
}

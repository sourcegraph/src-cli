package output

import (
	"time"
)

type progressWithStatusBarsSimple struct {
	*progressSimple

	statusBars []*StatusBar
}

func (p *progressWithStatusBarsSimple) Complete() {
	p.stop()
	writeBars(p.Output, p.bars)
	writeStatusBars(p.Output, p.statusBars)
}

func newProgressWithStatusBarsSimple(bars []*ProgressBar, statusBars []*StatusBar, o *Output) *progressWithStatusBarsSimple {
	p := &progressWithStatusBarsSimple{
		progressSimple: &progressSimple{
			Output: o,
			bars:   bars,
			done:   make(chan chan struct{}),
		},
		statusBars: statusBars,
	}

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if p.Output.opts.Verbose {
					writeBars(p.Output, p.bars)
				}

			case c := <-p.done:
				c <- struct{}{}
				return
			}
		}
	}()

	return p
}

func writeStatusBar(w Writer, bar *StatusBar) {
	w.Writef("%s: "+bar.format, append([]interface{}{bar.label}, bar.args...)...)
}

func writeStatusBars(o *Output, bars []*StatusBar) {
	if len(bars) > 1 {
		block := o.Block(Line("", StyleReset, "Status:"))
		defer block.Close()

		for _, bar := range bars {
			writeStatusBar(block, bar)
		}
	} else if len(bars) == 1 {
		writeStatusBar(o, bars[0])
	}
}

package watchdog

import (
	"time"
)

type WatchDog struct {
	interval time.Duration
	ticker   *time.Ticker
	done     chan struct{}
	callback func()
}

func New(interval time.Duration, callback func()) *WatchDog {
	t := time.NewTicker(interval)
	done := make(chan struct{}, 1)

	return &WatchDog{
		interval: interval,
		ticker:   t,
		done:     done,
		callback: callback,
	}
}

func (w *WatchDog) Stop() {
	close(w.done)
}

func (w *WatchDog) Start() {
	for {
		select {
		case <-w.ticker.C:
			go w.callback()
		case <-w.done:
			w.ticker.Stop()
		}
	}
}

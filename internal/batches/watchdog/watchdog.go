package watchdog

import (
	"time"

	"github.com/derision-test/glock"
)

type WatchDog struct {
	ticker   glock.Ticker
	callback func()
}

func New(interval time.Duration, callback func()) *WatchDog {
	ticker := glock.NewRealTicker(interval)
	return &WatchDog{
		ticker:   ticker,
		callback: callback,
	}
}

func (w *WatchDog) Stop() {
	w.ticker.Stop()
}

func (w *WatchDog) Start() {
	for _ = range w.ticker.Chan() {
		go w.callback()
	}
}

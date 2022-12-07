package watchdog

import (
	"time"
)

type Watcher interface {
	Start()
	Stop()
}

type watchDog struct {
	interval time.Duration
	ticker   *time.Ticker
	done     chan bool
	callback func()
}

func New(interval time.Duration, callback func()) Watcher {
	t := time.NewTicker(interval)
	done := make(chan bool)

	return &watchDog{
		interval: interval,
		ticker:   t,
		done:     done,
		callback: callback,
	}
}

func (w *watchDog) Stop() {
	close(w.done)
}

func (w *watchDog) Start() {
	for {
		select {
		case <-w.ticker.C:
			go w.callback()
		case <-w.done:
			w.ticker.Stop()
		}
	}
}

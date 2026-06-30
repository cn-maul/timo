package media

import (
	"sync"
	"time"
)

// Emitter is a function that sends media state to the frontend.
type Emitter func(info *MediaInfo)

// Poller periodically fetches media state and emits it to the frontend.
// It also supports an optional signal source (e.g. D-Bus PropertiesChanged)
// for instant updates when media state changes.
type Poller struct {
	provider   MediaProvider
	emitter    Emitter
	interval   time.Duration
	stop       chan struct{}
	lastInfo   *MediaInfo
	signalChan <-chan struct{}

	// Guard against double-Start and safe multi-Stop
	mu       sync.Mutex
	running  bool
	stopOnce sync.Once
}

// NewPoller creates a new Poller.
func NewPoller(provider MediaProvider, emitter Emitter, interval time.Duration) *Poller {
	return &Poller{
		provider: provider,
		emitter:  emitter,
		interval: interval,
		stop:     make(chan struct{}),
		lastInfo: &MediaInfo{},
	}
}

// SetSignalSource attaches an optional signal channel. When the channel fires,
// the poller immediately fetches and emits the latest state, bypassing the
// polling interval. This allows instant response to D-Bus PropertiesChanged
// signals while still having the polling loop as a fallback.
//
// Must be called before Start().
func (p *Poller) SetSignalSource(ch <-chan struct{}) {
	p.signalChan = ch
}

// Start begins the polling loop in a goroutine. It is a no-op if already running.
func (p *Poller) Start() {
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return
	}
	p.running = true
	p.stop = make(chan struct{})
	p.stopOnce = sync.Once{}
	p.mu.Unlock()

	go func() {
		ticker := time.NewTicker(p.interval)
		defer ticker.Stop()

		p.fetchAndEmit()

		for {
			select {
			case <-ticker.C:
				p.fetchAndEmit()
			case <-p.signalChan:
				// Signal received (e.g. D-Bus PropertiesChanged) — fetch immediately
				p.fetchAndEmit()
			case <-p.stop:
				return
			}
		}
	}()
}

// Stop halts the polling loop. Safe to call multiple times.
func (p *Poller) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.running {
		return
	}
	p.stopOnce.Do(func() {
		close(p.stop)
		p.running = false
	})
}

func (p *Poller) fetchAndEmit() {
	info, err := p.provider.GetState()
	if err != nil {
		if p.lastInfo.Title != "" {
			p.emitter(&MediaInfo{})
			p.lastInfo = &MediaInfo{}
		}
		return
	}

	if p.hasChanged(info) {
		p.emitter(info)
		p.lastInfo = info
	}
}

func (p *Poller) hasChanged(info *MediaInfo) bool {
	if p.lastInfo.Title != info.Title ||
		p.lastInfo.Artist != info.Artist ||
		p.lastInfo.Album != info.Album ||
		p.lastInfo.Playing != info.Playing ||
		p.lastInfo.CoverURL != info.CoverURL ||
		p.lastInfo.DurationMs != info.DurationMs {
		return true
	}
	diff := info.PositionMs - p.lastInfo.PositionMs
	// Emit if position jumped backward (e.g. track restart) or
	// advanced >500ms. With a 1s polling interval, normal playback
	// advances ~1000ms, so 500 reliably triggers updates without
	// causing excessive re-renders for very small movements.
	return diff < -1000 || diff > 500
}

// GetProvider returns the underlying MediaProvider.
func (p *Poller) GetProvider() MediaProvider {
	return p.provider
}

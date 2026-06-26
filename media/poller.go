package media

import "time"

// Emitter is a function that sends media state to the frontend.
type Emitter func(info *MediaInfo)

// Poller periodically fetches media state and emits it to the frontend.
type Poller struct {
	provider MediaProvider
	emitter  Emitter
	interval time.Duration
	stop     chan struct{}
	lastInfo *MediaInfo
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

// Start begins the polling loop in a goroutine.
func (p *Poller) Start() {
	go func() {
		ticker := time.NewTicker(p.interval)
		defer ticker.Stop()

		p.fetchAndEmit()

		for {
			select {
			case <-ticker.C:
				p.fetchAndEmit()
			case <-p.stop:
				return
			}
		}
	}()
}

// Stop halts the polling loop.
func (p *Poller) Stop() {
	close(p.stop)
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
		p.lastInfo.Playing != info.Playing ||
		p.lastInfo.CoverURL != info.CoverURL {
		return true
	}
	diff := info.PositionMs - p.lastInfo.PositionMs
	return diff < -1000 || diff > 2000
}

// GetProvider returns the underlying MediaProvider.
func (p *Poller) GetProvider() MediaProvider {
	return p.provider
}

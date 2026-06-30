package app

import (
	"errors"
	"timo/internal/media"
)

var errMediaServiceUnavailable = errors.New("media service unavailable: provider not initialized")

// MediaService exposes playback controls to the frontend via Wails bindings.
type MediaService struct {
	poller *media.Poller
}

func NewMediaService(poller *media.Poller) *MediaService {
	return &MediaService{poller: poller}
}

func (s *MediaService) Play() error {
	p := s.getProvider()
	if p == nil {
		return errMediaServiceUnavailable
	}
	return p.Play()
}

func (s *MediaService) Pause() error {
	p := s.getProvider()
	if p == nil {
		return errMediaServiceUnavailable
	}
	return p.Pause()
}

func (s *MediaService) Next() error {
	p := s.getProvider()
	if p == nil {
		return errMediaServiceUnavailable
	}
	return p.Next()
}

func (s *MediaService) Previous() error {
	p := s.getProvider()
	if p == nil {
		return errMediaServiceUnavailable
	}
	return p.Previous()
}

// getProvider returns the MediaProvider, or nil if the poller or provider is not initialized.
func (s *MediaService) getProvider() media.MediaProvider {
	if s.poller == nil {
		return nil
	}
	return s.poller.GetProvider()
}

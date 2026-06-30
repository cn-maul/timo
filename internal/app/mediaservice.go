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

// TogglePlayPause checks the current playing state from the provider
// and calls Play or Pause accordingly. Returns the new playing state.
func (s *MediaService) TogglePlayPause() (bool, error) {
	p := s.getProvider()
	if p == nil {
		return false, errMediaServiceUnavailable
	}
	// Get current state to determine what action to take
	state, err := p.GetState()
	if err != nil {
		// Fallback: try play
		if playErr := p.Play(); playErr != nil {
			return false, playErr
		}
		return true, nil
	}
	if state.Playing {
		if err := p.Pause(); err != nil {
			return true, err
		}
		return false, nil
	}
	if err := p.Play(); err != nil {
		return false, err
	}
	return true, nil
}

// getProvider returns the MediaProvider, or nil if the poller or provider is not initialized.
func (s *MediaService) getProvider() media.MediaProvider {
	if s.poller == nil {
		return nil
	}
	return s.poller.GetProvider()
}

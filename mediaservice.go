package main

import (
	"context"
	"timo/media"
)

// MediaService exposes playback controls to the frontend via Wails bindings.
type MediaService struct {
	ctx    context.Context
	poller *media.Poller
}

func NewMediaService(poller *media.Poller) *MediaService {
	return &MediaService{poller: poller}
}

func (s *MediaService) ServiceStartup(ctx context.Context) error {
	s.ctx = ctx
	return nil
}

func (s *MediaService) Play() error {
	return s.poller.GetProvider().Play()
}

func (s *MediaService) Pause() error {
	return s.poller.GetProvider().Pause()
}

func (s *MediaService) Next() error {
	return s.poller.GetProvider().Next()
}

func (s *MediaService) Previous() error {
	return s.poller.GetProvider().Previous()
}

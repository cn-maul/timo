package main

import (
	"log"
	"sync"
)

// SettingsService is a Wails Service that exposes settings read/write to the
// frontend and emits "settings-updated" events on every change.
type SettingsService struct {
	mu       sync.Mutex
	settings TimoSettings
}

// NewSettingsService loads persisted settings (or creates defaults) and
// returns a ready-to-use service.
func NewSettingsService() *SettingsService {
	s, err := LoadSettings()
	if err != nil {
		log.Printf("timo: failed to load settings, using defaults: %v", err)
		s = DefaultSettings()
		_ = SaveSettings(s)
	}
	log.Printf("timo: settings loaded from %s", GetSettingsPath())
	return &SettingsService{settings: s}
}

// Get returns the current in-memory settings snapshot.
func (s *SettingsService) Get() TimoSettings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.settings
}

// Update replaces the in-memory settings, persists them to disk, and emits a
// "settings-updated" event so the frontend and tray can react.
func (s *SettingsService) Update(settings TimoSettings) TimoSettings {
	s.mu.Lock()
	s.settings = settings
	s.mu.Unlock()

	if err := SaveSettings(settings); err != nil {
		log.Printf("timo: failed to save settings: %v", err)
	}

	if mainApp != nil {
		mainApp.Event.Emit("settings-updated", &settings)
	}

	return settings
}

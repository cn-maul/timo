package app

import (
	"log"
	"sync"
)

// SettingsService is a Wails Service that exposes settings read/write to the
// frontend and emits "settings-updated" events on every change.
type SettingsService struct {
	mu       sync.Mutex
	bus      EventBus
	settings TimoSettings
}

// NewSettingsService loads persisted settings (or creates defaults) and
// returns a ready-to-use service.
func NewSettingsService(bus EventBus) *SettingsService {
	s, err := LoadSettings()
	if err != nil {
		log.Printf("timo: failed to load settings, using defaults: %v", err)
		s = DefaultSettings()
		_ = SaveSettings(s)
	}
	log.Printf("timo: settings loaded from %s", GetSettingsPath())
	return &SettingsService{bus: bus, settings: s}
}

// Get returns the current in-memory settings snapshot.
func (s *SettingsService) Get() TimoSettings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.settings
}

// Update persists settings to disk first, then updates the in-memory copy so
// that a disk failure does not leave the UI showing unsaved settings. On
// failure it returns the previously-stored settings and does not emit the
// "settings-updated" event.
func (s *SettingsService) Update(settings TimoSettings) TimoSettings {
	s.mu.Lock()
	if err := SaveSettings(settings); err != nil {
		log.Printf("timo: failed to save settings: %v", err)
		old := s.settings
		s.mu.Unlock()
		return old
	}
	s.settings = settings
	s.mu.Unlock()

	if s.bus != nil {
		s.bus.Emit("settings-updated", &settings)
	}

	return settings
}

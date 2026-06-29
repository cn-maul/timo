package main

import (
	"encoding/json"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// registerSettingsEventHandlers sets up event-based communication so the
// frontend can read/write settings without requiring auto-generated RPC
// bindings.
func registerSettingsEventHandlers(app *application.App, svc *SettingsService) {
	// Frontend → "get-settings" → Backend responds with "settings-loaded"
	app.Event.On("get-settings", func(event *application.CustomEvent) {
		settings := svc.Get()
		app.Event.Emit("settings-loaded", &settings)
	})

	// Frontend → "save-settings" with data → Backend persists + broadcasts
	app.Event.On("save-settings", func(event *application.CustomEvent) {
		settings := parseSettings(event.Data)
		if settings != nil {
			svc.Update(*settings)
		} else {
			log.Printf("timo: save-settings received unhandled data type: %T", event.Data)
		}
	})
}

// parseSettings attempts to extract a TimoSettings from various types that
// Wails may deserialize from frontend Events.Emit calls.
func parseSettings(data interface{}) *TimoSettings {
	switch v := data.(type) {
	case *TimoSettings:
		return v
	case map[string]interface{}:
		return parseSettingsMap(v)
	}
	return nil
}

// parseSettingsMap converts a generic map (from frontend JSON) to TimoSettings.
func parseSettingsMap(m map[string]interface{}) *TimoSettings {
	s := DefaultSettings()

	if priorities, ok := m["displayPriority"].([]interface{}); ok {
		strs := make([]string, 0, len(priorities))
		for _, p := range priorities {
			if s, ok := p.(string); ok {
				strs = append(strs, s)
			}
		}
		if len(strs) > 0 {
			s.DisplayPriority = strs
		}
	}

	if idle, ok := m["idleDisplay"].(string); ok {
		s.IdleDisplay = idle
	}

	if theme, ok := m["theme"].(string); ok {
		s.Theme = theme
	}

	// New boolean display options
	if v, ok := m["showToolContext"].(bool); ok {
		s.ShowToolContext = v
	}
	if v, ok := m["showToolProgress"].(bool); ok {
		s.ShowToolProgress = v
	}
	if v, ok := m["showSubagentDetails"].(bool); ok {
		s.ShowSubagentDetails = v
	}

	// Hotkeys config
	if hkRaw, ok := m["hotkeys"]; ok {
		var rawHotkey struct {
			Enabled bool `json:"enabled"`
			ToggleWindow string `json:"toggleWindow"`
			ToggleMedia string `json:"toggleMedia"`
		}
		hkJSON, err := json.Marshal(hkRaw)
		if err == nil && string(hkJSON) != "null" {
			json.Unmarshal(hkJSON, &rawHotkey)
			s.Hotkeys.Enabled = rawHotkey.Enabled
			s.Hotkeys.ToggleWindow = rawHotkey.ToggleWindow
			s.Hotkeys.ToggleMedia = rawHotkey.ToggleMedia
		} else if !rawHotkey.Enabled {
			s.Hotkeys.Enabled = true // default on for backward compatibility
		}
	}

	return &s
}

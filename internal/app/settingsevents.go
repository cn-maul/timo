package app

import (
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

// validIdleDisplay returns true if the given idleDisplay value is valid.
func validIdleDisplay(v string) bool {
	switch v {
	case "all", "cpu", "mem", "net", "none":
		return true
	}
	return false
}

// validTheme returns true if the given theme value is valid.
func validTheme(v string) bool {
	switch v {
	case "dark", "light", "frosted":
		return true
	}
	return false
}

// validDisplayPriorityItem returns true if a displayPriority entry is valid.
func validDisplayPriorityItem(v string) bool {
	switch v {
	case "ai", "media":
		return true
	}
	return false
}

// parseSettingsMap converts a generic map (from frontend JSON) to TimoSettings.
func parseSettingsMap(m map[string]interface{}) *TimoSettings {
	s := DefaultSettings()

	if priorities, ok := m["displayPriority"].([]interface{}); ok {
		strs := make([]string, 0, len(priorities))
		for _, p := range priorities {
			if str, ok := p.(string); ok && validDisplayPriorityItem(str) {
				strs = append(strs, str)
			}
		}
		if len(strs) > 0 {
			s.DisplayPriority = strs
		}
	}

	if idle, ok := m["idleDisplay"].(string); ok && validIdleDisplay(idle) {
		s.IdleDisplay = idle
	}

	if theme, ok := m["theme"].(string); ok && validTheme(theme) {
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

	// Net unit
	if netUnit, ok := m["netUnit"].(string); ok {
		switch netUnit {
		case "auto", "kb", "mb":
			s.NetUnit = netUnit
		}
	}

	return &s
}

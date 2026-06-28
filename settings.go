package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
)

// TimoSettings holds all user-configurable settings for Timo.
type TimoSettings struct {
	// DisplayPriority controls which activity mode takes precedence.
	// Order matters: first entry = highest priority.
	// Valid values: "ai", "media"
	DisplayPriority []string `json:"displayPriority"`

	// IdleDisplay controls what is shown when no activity is detected.
	// Valid values: "all" | "cpu" | "mem" | "none"
	IdleDisplay string `json:"idleDisplay"`

	// Theme selects the UI color scheme.
	// Valid values: "dark" | "light"
	Theme string `json:"theme"`
}

// DefaultSettings returns the factory-default settings.
func DefaultSettings() TimoSettings {
	return TimoSettings{
		DisplayPriority: []string{"ai", "media"},
		IdleDisplay:     "all",
		Theme:           "dark",
	}
}

// GetSettingsPath returns the platform-specific config file path.
// Linux:   ~/.config/timo/settings.json
// Windows: %APPDATA%\timo\settings.json
func GetSettingsPath() string {
	var configDir string
	switch runtime.GOOS {
	case "windows":
		configDir = os.Getenv("APPDATA")
	default:
		configDir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(configDir, "timo", "settings.json")
}

// LoadSettings reads settings from disk. If the file does not exist it
// returns DefaultSettings without error. Missing fields are filled with
// defaults so old configs remain compatible.
func LoadSettings() (TimoSettings, error) {
	path := GetSettingsPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultSettings(), nil
		}
		return DefaultSettings(), err
	}

	var s TimoSettings
	if err := json.Unmarshal(data, &s); err != nil {
		return DefaultSettings(), err
	}

	// Field-level defaults for forward compatibility
	def := DefaultSettings()
	if s.DisplayPriority == nil {
		s.DisplayPriority = def.DisplayPriority
	}
	if s.IdleDisplay == "" {
		s.IdleDisplay = def.IdleDisplay
	}
	if s.Theme == "" {
		s.Theme = def.Theme
	}
	return s, nil
}

// SaveSettings writes settings to disk, creating parent directories as needed.
func SaveSettings(s TimoSettings) error {
	path := GetSettingsPath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

package app

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
	// Valid values: "all" | "cpu" | "mem" | "net" | "none"
	IdleDisplay string `json:"idleDisplay"`

	// Theme selects the UI color scheme.
	// Valid values: "dark" | "light" | "frosted"
	Theme string `json:"theme"`

	// ShowToolContext controls whether to show tool operation details.
	// When true, displays file paths, command summaries, etc.
	ShowToolContext bool `json:"showToolContext"`

	// ShowToolProgress controls whether to show tool call count progress.
	// When true, displays progress bar based on tool count.
	ShowToolProgress bool `json:"showToolProgress"`

	// ShowSubagentDetails controls whether to show subagent type and description.
	// When true, displays agent type (Explore, Plan, etc.) and task description.
	ShowSubagentDetails bool `json:"showSubagentDetails"`

	// NetUnit controls how network speeds are displayed.
	// Valid values: "auto" | "kb" | "mb"
	NetUnit string `json:"netUnit,omitempty"`
}

// DefaultSettings returns the factory-default settings.
func DefaultSettings() TimoSettings {
	return TimoSettings{
		DisplayPriority:    []string{"ai", "media"},
		IdleDisplay:        "all",
		Theme:              "dark",
		ShowToolContext:    true,
		ShowToolProgress:   true,
		ShowSubagentDetails: true,
		NetUnit:            "auto",
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

	// Parse raw JSON once to check which fields exist (avoids re-parsing per field).
	var rawFields map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawFields); err != nil {
		return DefaultSettings(), err
	}
	fieldExists := func(name string) bool {
		_, ok := rawFields[name]
		return ok
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
	// New boolean fields default to true if not present
	if !fieldExists("showToolContext") {
		s.ShowToolContext = def.ShowToolContext
	}
	if !fieldExists("showToolProgress") {
		s.ShowToolProgress = def.ShowToolProgress
	}
	if !fieldExists("showSubagentDetails") {
		s.ShowSubagentDetails = def.ShowSubagentDetails
	}
	// NetUnit default
	if !fieldExists("netUnit") {
		s.NetUnit = def.NetUnit
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
	return os.WriteFile(path, data, 0600)
}

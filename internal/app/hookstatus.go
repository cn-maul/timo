//go:build linux

package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// HooksStatus represents the installation status of hooks for each tool.
type HooksStatus struct {
	Claude   HookInfo `json:"claude"`
	Reasonix HookInfo `json:"reasonix"`
}

type HookInfo struct {
	Installed    bool   `json:"installed"`
	Path         string `json:"path"`
	PathMismatch bool   `json:"pathMismatch,omitempty"`  // hooks exist but point to different timo
	CurrentPath  string `json:"currentPath,omitempty"`   // current timo binary path
}

// getHooksStatus checks whether hooks are installed for Claude Code and Reasonix.
func getHooksStatus() HooksStatus {
	return HooksStatus{
		Claude:   checkHookInstalled(".claude"),
		Reasonix: checkHookInstalled(".reasonix"),
	}
}

// checkHookInstalled reads the settings file and checks for timo hooks.
// Also checks if the hooks point to the current timo binary path.
func checkHookInstalled(configDir string) HookInfo {
	home, err := os.UserHomeDir()
	if err != nil {
		return HookInfo{}
	}
	settingsPath := filepath.Join(home, configDir, "settings.json")

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return HookInfo{Path: settingsPath}
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return HookInfo{Path: settingsPath}
	}

	installed := false
	pathMismatch := false
	currentTimoPath := ""

	// Get current timo binary path
	if execPath, err := os.Executable(); err == nil {
		if absPath, err := filepath.Abs(execPath); err == nil {
			currentTimoPath = absPath
		}
	}

	if hooksRaw, ok := settings["hooks"]; ok {
		hooksJSON, _ := json.Marshal(hooksRaw)
		hooksStr := string(hooksJSON)
		if strings.Contains(hooksStr, "timo notify") {
			installed = true
			// Check if the path matches current timo
			if currentTimoPath != "" && !strings.Contains(hooksStr, currentTimoPath) {
				pathMismatch = true
			}
		}
	}

	return HookInfo{
		Installed:    installed,
		Path:         settingsPath,
		PathMismatch: pathMismatch,
		CurrentPath:  currentTimoPath,
	}
}

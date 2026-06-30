//go:build linux

package app

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// isTimoHook returns true if a hook command entry contains "timo notify".
func isTimoHook(entry map[string]interface{}) bool {
	cmd, _ := entry["command"].(string)
	return strings.Contains(cmd, "timo notify")
}

// setupHooks is the shared implementation for RunSetup and AutoSetupHooks.
func setupHooks(isAuto bool) (string, error) {
	return installHooks(isAuto, ".claude", "claude", msgAutoConfigSuccess)
}

// setupReasonixHooks configures Reasonix hooks in ~/.reasonix/settings.json.
func setupReasonixHooks(isAuto bool) (string, error) {
	return installHooks(isAuto, ".reasonix", "reasonix", msgReasonixAutoConfigSuccess)
}

// AutoSetupHooks checks if Claude Code hooks are configured, and injects them if not.
func AutoSetupHooks() {
	timoPath, err := setupHooks(true)
	if err != nil {
		log.Printf("AutoSetupHooks: %v", err)
		return
	}
	if timoPath != "" {
		log.Println(msgAutoConfigSuccess)
	}
}

// AutoSetupReasonixHooks checks if Reasonix hooks are configured, and injects them if not.
func AutoSetupReasonixHooks() {
	timoPath, err := setupReasonixHooks(true)
	if err != nil {
		log.Printf("AutoSetupReasonixHooks: %v", err)
		return
	}
	if timoPath != "" {
		log.Println(msgReasonixAutoConfigSuccess)
	}
}

// buildHooksConfig builds the timo hooks entries for the given type prefix.
// Uses UserPromptSubmit (not PreToolUse) as the primary "work started" signal.
// Reasonix uses flat format: {command}; Claude uses nested format: {matcher, hooks: [{type, command}]}.
func buildHooksConfig(timoPath string, typePrefix string) map[string]interface{} {
	if typePrefix == "reasonix" {
		// Reasonix flat format per docs/reasonix.md spec
		// 9 events (no SessionEnd - ProcessMonitor handles process exit)
		return map[string]interface{}{
			"SessionStart": []interface{}{
				map[string]interface{}{
					"command":     timoPath + ` notify --type reasonix-session-start`,
					"description": "Timo: session started",
				},
			},
			"UserPromptSubmit": []interface{}{
				map[string]interface{}{
					"command":     timoPath + ` notify --type reasonix-prompt --dir "$(pwd)"`,
					"description": "Timo: prompt submitted",
				},
			},
			"PreToolUse": []interface{}{
				map[string]interface{}{
					"match":       "*",
					"command":     timoPath + ` notify --type reasonix-pre-tool`,
					"description": "Timo: tool starting",
					"timeout":     5000,
				},
			},
			"PostToolUse": []interface{}{
				map[string]interface{}{
					"match":       "*",
					"command":     timoPath + ` notify --type reasonix-tool`,
					"description": "Timo: tool finished",
					"timeout":     30000,
				},
			},
			"PostLLMCall": []interface{}{
				map[string]interface{}{
					"command":     timoPath + ` notify --type reasonix-llm`,
					"description": "Timo: LLM response",
					"timeout":     30000,
				},
			},
			"SubagentStop": []interface{}{
				map[string]interface{}{
					"command":     timoPath + ` notify --type reasonix-subagent-stop`,
					"description": "Timo: subagent done",
				},
			},
			"Stop": []interface{}{
				map[string]interface{}{
					"command":     timoPath + ` notify --type reasonix-stop`,
					"description": "Timo: turn finished",
				},
			},
			"Notification": []interface{}{
				map[string]interface{}{
					"command":     timoPath + ` notify --type reasonix-notify`,
					"description": "Timo: attention needed",
				},
			},
			"PreCompact": []interface{}{
				map[string]interface{}{
					"command":     timoPath + ` notify --type reasonix-precompact`,
					"description": "Timo: compacting",
					"timeout":     30000,
				},
			},
		}
	}
	// Claude Code nested format (matches reasonix event set)
	// ProcessMonitor serves as backup for process exit detection.
	return map[string]interface{}{
		"UserPromptSubmit": []interface{}{
			map[string]interface{}{
				"matcher": "",
				"hooks": []interface{}{
					map[string]interface{}{
						"type":    "command",
						"command": timoPath + ` notify --type claude-prompt --dir "$(pwd)"`,
					},
				},
			},
		},
		"PreToolUse": []interface{}{
			map[string]interface{}{
				"matcher": "*",
				"hooks": []interface{}{
					map[string]interface{}{
						"type":    "command",
						"command": timoPath + ` notify --type claude-pre-tool`,
					},
				},
			},
		},
		"PostToolUse": []interface{}{
			map[string]interface{}{
				"matcher": "*",
				"hooks": []interface{}{
					map[string]interface{}{
						"type":    "command",
						"command": timoPath + ` notify --type claude-tool`,
					},
				},
			},
		},
		"SubagentStart": []interface{}{
			map[string]interface{}{
				"matcher": "",
				"hooks": []interface{}{
					map[string]interface{}{
						"type":    "command",
						"command": timoPath + ` notify --type claude-subagent`,
					},
				},
			},
		},
		"SubagentStop": []interface{}{
			map[string]interface{}{
				"matcher": "",
				"hooks": []interface{}{
					map[string]interface{}{
						"type":    "command",
						"command": timoPath + ` notify --type claude-subagent-stop`,
					},
				},
			},
		},
		"Stop": []interface{}{
			map[string]interface{}{
				"matcher": "",
				"hooks": []interface{}{
					map[string]interface{}{
						"type":    "command",
						"command": timoPath + ` notify --type claude-stop`,
					},
				},
			},
		},
		"Notification": []interface{}{
			map[string]interface{}{
				"matcher": "",
				"hooks": []interface{}{
					map[string]interface{}{
						"type":    "command",
						"command": timoPath + ` notify --type claude-notify --msg "` + msgConfirmNeeded + `"`,
					},
				},
			},
		},
	}
}

// installHooks is the shared implementation for setupHooks and setupReasonixHooks.
// When isAuto is true, it skips configuration if timo hooks already exist.
// It merges timo hooks into existing settings, preserving any non-timo hooks.
func installHooks(isAuto bool, configDir string, typePrefix string, successMsg string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	configPath := filepath.Join(home, configDir)
	settingsPath := filepath.Join(configPath, "settings.json")

	timoPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("cannot determine timo path: %w", err)
	}
	absPath, err := filepath.Abs(timoPath)
	if err != nil {
		return "", fmt.Errorf("cannot resolve timo path: %w", err)
	}
	timoPath = absPath

	// Read existing settings
	var settings map[string]interface{}
	data, err := os.ReadFile(settingsPath)
	if err == nil {
		// Validate that the file is valid JSON before proceeding
		if err := json.Unmarshal(data, &settings); err != nil {
			return "", fmt.Errorf("%s contains invalid JSON: %w; please fix or delete the file", settingsPath, err)
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("cannot read %s: %w", settingsPath, err)
	}
	if settings == nil {
		settings = make(map[string]interface{})
	}

	// In auto mode, skip if timo hooks are already configured
	if isAuto {
		if hooksRaw, ok := settings["hooks"]; ok {
			hooksJSON, err := json.Marshal(hooksRaw)
			if err != nil {
				log.Printf("Warning: cannot marshal existing hooks: %v", err)
			} else if strings.Contains(string(hooksJSON), "timo notify") {
				return "", nil // already configured
			}
		}
	}

	// Build the desired timo hooks config (format depends on target tool)
	timoHooks := buildHooksConfig(timoPath, typePrefix)

	// Merge: preserve non-timo hooks, replace timo hooks for each event type.
	// For any event type not covered by timoHooks, existing entries are kept.
	// Reasonix uses flat format {match, command}; Claude uses nested {matcher, hooks: [{type, command}]}.
	isReasonix := typePrefix == "reasonix"
	merged := make(map[string]interface{})

	// helper: safely get or create a []interface{} from merged[event]
	getSlice := func(key string) []interface{} {
		if v, ok := merged[key].([]interface{}); ok {
			return v
		}
		return nil
	}

	if existingHooks, ok := settings["hooks"].(map[string]interface{}); ok {
		for event, existingRaw := range existingHooks {
			if _, ok := timoHooks[event]; ok {
				// This event type is managed by timo; filter out old timo entries.
				existingArr, _ := existingRaw.([]interface{})
				for _, entry := range existingArr {
					m, ok := entry.(map[string]interface{})
					if !ok {
						continue
					}
					if isReasonix {
						// Flat format: entry directly has {match, command}
						if isTimoHook(m) {
							continue // skip old timo entry
						}
						merged[event] = append(getSlice(event), m)
					} else {
						// Nested format: entry has {matcher, hooks: [{type, command}]}
						entries, _ := m["hooks"].([]interface{})
						kept := make([]interface{}, 0, len(entries))
						for _, h := range entries {
							if hookMap, ok := h.(map[string]interface{}); ok && !isTimoHook(hookMap) {
								kept = append(kept, h)
							}
						}
						if len(kept) > 0 {
							m["hooks"] = kept
							merged[event] = append(getSlice(event), m)
						}
					}
				}
			} else {
				// Event type not managed by timo; keep it as-is.
				merged[event] = existingRaw
			}
		}
	}
	// Append the timo hook entries for each managed event type.
	for event, timoEntries := range timoHooks {
		merged[event] = append(getSlice(event), timoEntries.([]interface{})...)
	}
	settings["hooks"] = merged

	// Write back
	if err := os.MkdirAll(configPath, 0755); err != nil {
		return "", fmt.Errorf("cannot create %s: %w", configPath, err)
	}
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return "", fmt.Errorf("cannot encode settings: %w", err)
	}
	if err := os.WriteFile(settingsPath, append(out, '\n'), 0644); err != nil {
		return "", fmt.Errorf("cannot write %s: %w", settingsPath, err)
	}

	return timoPath, nil
}

// removeHooks removes all timo-related hooks from the specified config directory.
func removeHooks(configDir string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}
	settingsPath := filepath.Join(home, configDir, "settings.json")

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No settings file, nothing to remove
		}
		return fmt.Errorf("cannot read %s: %w", settingsPath, err)
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return fmt.Errorf("cannot parse %s: %w", settingsPath, err)
	}

	if settings == nil {
		return nil
	}

	hooks, ok := settings["hooks"].(map[string]interface{})
	if !ok {
		return nil
	}

	for event, entriesRaw := range hooks {
		entries, ok := entriesRaw.([]interface{})
		if !ok {
			continue
		}
		var kept []interface{}
		for _, entry := range entries {
			m, ok := entry.(map[string]interface{})
			if !ok {
				kept = append(kept, entry)
				continue
			}
			if isTimoHookNested(m) {
				continue
			}
			kept = append(kept, entry)
		}
		if len(kept) > 0 {
			hooks[event] = kept
		} else {
			delete(hooks, event)
		}
	}

	if len(hooks) == 0 {
		delete(settings, "hooks")
	} else {
		settings["hooks"] = hooks
	}

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("cannot encode settings: %w", err)
	}
	return os.WriteFile(settingsPath, append(out, '\n'), 0644)
}

// isTimoHookNested checks if a hook entry contains timo notify.
func isTimoHookNested(m map[string]interface{}) bool {
	if cmd, ok := m["command"].(string); ok && strings.Contains(cmd, "timo notify") {
		return true
	}
	if hooksArr, ok := m["hooks"].([]interface{}); ok {
		for _, h := range hooksArr {
			if hookMap, ok := h.(map[string]interface{}); ok {
				if cmd, ok := hookMap["command"].(string); ok && strings.Contains(cmd, "timo notify") {
					return true
				}
			}
		}
	}
	return false
}

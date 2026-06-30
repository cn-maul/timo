package app

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ── Constants ──

const (
	msgTaskComplete               = "任务完成"
	msgConfirmNeeded              = "需要确认"
	msgAutoConfigSuccess          = "✓ 已自动配置 Claude Code hooks，请重启 Claude Code 使配置生效"
	msgReasonixAutoConfigSuccess  = "✓ 已自动配置 Reasonix hooks，请重启 Reasonix 使配置生效"
)

// ── Notification struct (cross-platform) ──

// Notification represents a message from external tools.
type Notification struct {
	Type    string `json:"type"`    // "claude-prompt"/"reasonix-prompt", "claude-tool"/"reasonix-tool", "claude-subagent"/"reasonix-subagent", "claude-subagent-done", "claude-notify"/"reasonix-notify", "claude-done"/"reasonix-done"
	Message string `json:"message"` // Human-readable context
	Tool    string `json:"tool"`    // Current tool name (from PreToolUse)
	WorkDir string `json:"workDir"` // Working directory
	Topic   string `json:"topic"`   // User's prompt text (from UserPromptSubmit)

	// Extended fields for richer display
	ToolInput   map[string]interface{} `json:"toolInput,omitempty"`   // Full tool parameters (from PreToolUse)
	ToolOutput  map[string]interface{} `json:"toolOutput,omitempty"`  // Tool result (from PostToolUse)
	DurationMs  int                    `json:"durationMs,omitempty"`  // Execution time in milliseconds
	AgentType   string                 `json:"agentType,omitempty"`   // Subagent type (Explore, Plan, etc.)
	AgentDesc   string                 `json:"agentDesc,omitempty"`   // Subagent task description
	AgentResult string                 `json:"agentResult,omitempty"` // Subagent result summary (last_assistant_message)
	FinalMsg    string                 `json:"finalMsg,omitempty"`    // Stop event summary message
	ToolCount   int                    `json:"toolCount,omitempty"`   // Tool call count for progress
	EffortLevel string                 `json:"effortLevel,omitempty"` // Effort level (low/medium/high/xhigh/max)
	IsPreTool   bool                   `json:"isPreTool,omitempty"`   // Is this a PreToolUse event (before execution)
}

// ── Hooks status structs ──

// HooksStatus represents the installation status of hooks for each tool.
type HooksStatus struct {
	Claude   HookInfo `json:"claude"`
	Reasonix HookInfo `json:"reasonix"`
}

// HookInfo represents the installation status of hooks for a single tool.
type HookInfo struct {
	Installed    bool   `json:"installed"`
	Path         string `json:"path"`
	PathMismatch bool   `json:"pathMismatch,omitempty"`  // hooks exist but point to different timo
	CurrentPath  string `json:"currentPath,omitempty"`   // current timo binary path
}

// ── Hook detection ──

// isTimoHook returns true if a hook command entry contains "timo notify".
func isTimoHook(entry map[string]interface{}) bool {
	cmd, _ := entry["command"].(string)
	return strings.Contains(cmd, "timo notify")
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

// ── Hooks status ──

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
	usesBasename := false // true = commands use "timo" (relying on PATH)
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
			// Check if the hook commands use a bare "timo" (basename, PATH-resolved)
			// or an absolute path.
			if strings.Contains(hooksStr, "\"timo notify") {
				usesBasename = true
			}
			if usesBasename {
				// Basename-only commands rely on PATH; no path mismatch possible
				pathMismatch = false
			} else if currentTimoPath != "" && !strings.Contains(hooksStr, currentTimoPath) {
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

// ── Hook configuration building ──

// buildHooksConfig builds the timo hooks entries for the given type prefix.
// hookCmd is the actual command string to embed in hook entries (e.g. "timo" or "/usr/bin/timo").
// Uses UserPromptSubmit (not PreToolUse) as the primary "work started" signal.
// Reasonix uses flat format: {command}; Claude uses nested format: {matcher, hooks: [{type, command}]}.
func buildHooksConfig(hookCmd string, typePrefix string) map[string]interface{} {
	if typePrefix == "reasonix" {
		// Reasonix flat format per docs/reasonix.md spec
		// 9 events (no SessionEnd - ProcessMonitor handles process exit)
		return map[string]interface{}{
			"SessionStart": []interface{}{
				map[string]interface{}{
					"command":     hookCmd + ` notify --type reasonix-session-start`,
					"description": "Timo: session started",
				},
			},
			"UserPromptSubmit": []interface{}{
				map[string]interface{}{
					"command":     hookCmd + ` notify --type reasonix-prompt --dir "$(pwd)"`,
					"description": "Timo: prompt submitted",
				},
			},
			"PreToolUse": []interface{}{
				map[string]interface{}{
					"match":       "*",
					"command":     hookCmd + ` notify --type reasonix-pre-tool`,
					"description": "Timo: tool starting",
					"timeout":     5000,
				},
			},
			"PostToolUse": []interface{}{
				map[string]interface{}{
					"match":       "*",
					"command":     hookCmd + ` notify --type reasonix-tool`,
					"description": "Timo: tool finished",
					"timeout":     30000,
				},
			},
			"PostLLMCall": []interface{}{
				map[string]interface{}{
					"command":     hookCmd + ` notify --type reasonix-llm`,
					"description": "Timo: LLM response",
					"timeout":     30000,
				},
			},
			"SubagentStop": []interface{}{
				map[string]interface{}{
					"command":     hookCmd + ` notify --type reasonix-subagent-stop`,
					"description": "Timo: subagent done",
				},
			},
			"Stop": []interface{}{
				map[string]interface{}{
					"command":     hookCmd + ` notify --type reasonix-stop`,
					"description": "Timo: turn finished",
				},
			},
			"Notification": []interface{}{
				map[string]interface{}{
					"command":     hookCmd + ` notify --type reasonix-notify`,
					"description": "Timo: attention needed",
				},
			},
			"PreCompact": []interface{}{
				map[string]interface{}{
					"command":     hookCmd + ` notify --type reasonix-precompact`,
					"description": "Timo: compacting",
					"timeout":     30000,
				},
			},
		}
	}
	// Claude Code nested format
	// ProcessMonitor serves as backup for process exit detection.
	return map[string]interface{}{
		"UserPromptSubmit": []interface{}{
			map[string]interface{}{
				"matcher": "",
				"hooks": []interface{}{
					map[string]interface{}{
						"type":    "command",
						"command": hookCmd + ` notify --type claude-prompt --dir "$(pwd)"`,
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
						"command": hookCmd + ` notify --type claude-pre-tool`,
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
						"command": hookCmd + ` notify --type claude-tool`,
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
						"command": hookCmd + ` notify --type claude-subagent`,
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
						"command": hookCmd + ` notify --type claude-subagent-stop`,
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
						"command": hookCmd + ` notify --type claude-stop`,
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
						"command": hookCmd + ` notify --type claude-notify --msg "` + msgConfirmNeeded + `"`,
					},
				},
			},
		},
	}
}

// ── Hook install / remove ──

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

	// Prefer just the executable name if timo is in PATH — this makes hooks
	// resilient to binary relocation and avoids fragile hardcoded paths.
	hookCommand := timoPath // fallback: absolute path
	if lookPath, lookupErr := exec.LookPath("timo"); lookupErr == nil {
		if absLookup, lookupErr := filepath.Abs(lookPath); lookupErr == nil {
			if absLookup == timoPath {
				// The binary resolved from PATH matches our own path, so using
				// just the basename is safe and portable.
				hookCommand = "timo"
			} else {
				// PATH finds a different timo than our own — that's suspicious.
				// Log a warning and use the PATH version (more likely to work
				// in hook subshells).
				log.Printf("timo: hook path mismatch — self=%s, PATH=%s; using PATH version", timoPath, absLookup)
				hookCommand = "timo"
				timoPath = absLookup
			}
		}
	} else {
		// timo not in PATH; verify the absolute path actually exists
		if _, err := os.Stat(timoPath); err != nil {
			log.Printf("timo: warning — hook path %s does not exist and timo not found in PATH", timoPath)
		}
	}

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
	// Use hookCommand (basename or PATH-resolved name) for the actual hook
	// entries, not the absolute timoPath — this avoids fragile hardcoded paths.
	timoHooks := buildHooksConfig(hookCommand, typePrefix)

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

// readStdinHookData reads JSON hook data from stdin (if piped) and merges
// it into the notification. This is called by RunCLI on both Linux and Windows.
func readStdinHookData(notif *Notification) {
	stat, err := os.Stdin.Stat()
	if err != nil {
		log.Printf("Error: cannot stat stdin: %v", err)
		return
	}
	if stat == nil {
		log.Println("Error: stdin stat returned nil")
		return
	}
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		data, err := io.ReadAll(io.LimitReader(os.Stdin, 262144)) // 256KB limit per spec
		if err == nil && len(data) > 0 {
			var hookData map[string]interface{}
			if json.Unmarshal(data, &hookData) == nil {
				parseHookData(notif, hookData)
			}
		}
	}
}

// parseHookData extracts fields from hook data into the notification.
// This is cross-platform shared logic called by RunCLI on both Linux and Windows.
func parseHookData(notif *Notification, hookData map[string]interface{}) {
	// Tool name: Claude uses "tool_name" (snake_case), Reasonix uses "toolName" (camelCase)
	if notif.Tool == "" {
		if tn, ok := hookData["tool_name"].(string); ok {
			notif.Tool = tn
		} else if tn, ok := hookData["toolName"].(string); ok {
			notif.Tool = tn
		}
	}
	// Message from hook payload (e.g. Notification event)
	if msg, ok := hookData["message"].(string); ok && notif.Message == "" {
		notif.Message = msg
	}
	// User prompt from UserPromptSubmit event (both tools use "prompt")
	if prompt, ok := hookData["prompt"].(string); ok && prompt != "" && notif.Topic == "" {
		if len(prompt) > 500 {
			notif.Topic = prompt[:500]
		} else {
			notif.Topic = prompt
		}
	}
	// Working directory from Reasonix payload (has "cwd" field)
	if cwd, ok := hookData["cwd"].(string); ok && cwd != "" && notif.WorkDir == "" {
		notif.WorkDir = cwd
	}

	// === Extended fields ===

	// Tool input (full parameters)
	// Reasonix spec uses "toolArgs" (camelCase); Claude uses "tool_input"/"toolInput"
	if toolArgs, ok := hookData["toolArgs"].(map[string]interface{}); ok {
		notif.ToolInput = toolArgs
	} else if toolInput, ok := hookData["tool_input"].(map[string]interface{}); ok {
		notif.ToolInput = toolInput
	} else if toolInput, ok := hookData["toolInput"].(map[string]interface{}); ok {
		notif.ToolInput = toolInput
	}

	// Tool output/result (from PostToolUse)
	// Reasonix spec uses "toolResult" (string); Claude uses "tool_response" (map)
	if toolResult, ok := hookData["toolResult"].(string); ok && toolResult != "" {
		// Wrap string result in a map for consistent frontend handling
		notif.ToolOutput = map[string]interface{}{"output": toolResult}
	} else if toolOutput, ok := hookData["tool_response"].(map[string]interface{}); ok {
		notif.ToolOutput = toolOutput
	}

	// Duration in milliseconds
	if duration, ok := hookData["duration_ms"].(float64); ok {
		notif.DurationMs = int(duration)
	}

	// Agent type (from SubagentStart/SubagentStop)
	if agentType, ok := hookData["agent_type"].(string); ok {
		notif.AgentType = agentType
	}

	// Agent description (from Agent tool input)
	if notif.ToolInput != nil {
		if desc, ok := notif.ToolInput["description"].(string); ok {
			notif.AgentDesc = desc
		} else if prompt, ok := notif.ToolInput["prompt"].(string); ok && len(prompt) > 0 {
			// Use prompt as description if no description field
			if len(prompt) > 100 {
				notif.AgentDesc = prompt[:100] + "..."
			} else {
				notif.AgentDesc = prompt
			}
		}
	}

	// Agent result / last assistant message (from SubagentStop/Stop)
	// Reasonix spec uses "lastAssistantText"; Claude uses "last_assistant_message"
	var lastAssistantText string
	if lat, ok := hookData["lastAssistantText"].(string); ok {
		lastAssistantText = lat
	} else if lastMsg, ok := hookData["last_assistant_message"].(string); ok {
		lastAssistantText = lastMsg
	}
	if lastAssistantText != "" {
		if len(lastAssistantText) > 200 {
			notif.AgentResult = lastAssistantText[:200] + "..."
		} else {
			notif.AgentResult = lastAssistantText
		}
		// Also use for Stop event's FinalMsg
		if strings.HasPrefix(notif.Type, "reasonix-stop") || strings.HasPrefix(notif.Type, "claude-stop") {
			if len(lastAssistantText) > 200 {
				notif.FinalMsg = lastAssistantText[:200] + "..."
			} else {
				notif.FinalMsg = lastAssistantText
			}
		}
	}

	// Effort level
	if effort, ok := hookData["effort"].(map[string]interface{}); ok {
		if level, ok := effort["level"].(string); ok {
			notif.EffortLevel = level
		}
	}
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

// RunSetup configures Claude Code hooks globally in ~/.claude/settings.json.
func RunSetup() {
	timoPath, err := setupHooks(false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	home, _ := os.UserHomeDir()
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	fmt.Printf("✓ Claude Code hooks configured in %s\n", settingsPath)
	fmt.Printf("  Timo path: %s\n", timoPath)
	fmt.Printf("  Hooks: UserPromptSubmit, Stop, Notification\n")
}

// RunSetupReasonix configures Reasonix hooks globally in ~/.reasonix/settings.json.
func RunSetupReasonix() {
	timoPath, err := setupReasonixHooks(false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	home, _ := os.UserHomeDir()
	settingsPath := filepath.Join(home, ".reasonix", "settings.json")
	fmt.Printf("✓ Reasonix hooks configured in %s\n", settingsPath)
	fmt.Printf("  Timo path: %s\n", timoPath)
	fmt.Printf("  Hooks: SessionStart, UserPromptSubmit, PreToolUse, PostToolUse, PostLLMCall, SubagentStop, Stop, Notification, PreCompact\n")
}

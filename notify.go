//go:build linux

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const sockPath = "/tmp/timo.sock"

// Chinese string constants for notifications and messages.
const (
	msgTaskComplete        = "任务完成"
	msgConfirmNeeded       = "需要确认"
	msgAutoConfigSuccess   = "✓ 已自动配置 Claude Code hooks，请重启 Claude Code 使配置生效"
	msgReasonixAutoConfigSuccess = "✓ 已自动配置 Reasonix hooks，请重启 Reasonix 使配置生效"
)

// Notification represents a message from external tools.
type Notification struct {
	Type    string `json:"type"`    // "claude-prompt"/"reasonix-prompt", "claude-done"/"reasonix-done", "claude-notify"/"reasonix-notify"
	Message string `json:"message"` // Human-readable context
	Tool    string `json:"tool"`    // Current tool name (from PreToolUse)
	WorkDir string `json:"workDir"` // Working directory
	Topic   string `json:"topic"`   // User's prompt text (from UserPromptSubmit)
}

// NotifyServer listens on a Unix domain socket for notifications.
type NotifyServer struct {
	listener net.Listener
	callback func(Notification)
	stopCh   chan struct{}
}

func NewNotifyServer(callback func(Notification)) *NotifyServer {
	return &NotifyServer{
		callback: callback,
		stopCh:   make(chan struct{}),
	}
}

func (s *NotifyServer) Start() error {
	os.Remove(sockPath)
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", sockPath, err)
	}
	// Set permissions immediately after socket creation to minimize exposure
	os.Chmod(sockPath, 0600)
	s.listener = ln

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				select {
				case <-s.stopCh:
					return
				default:
					continue
				}
			}
			go s.handleConn(conn)
		}
	}()

	return nil
}

func (s *NotifyServer) handleConn(conn net.Conn) {
	defer conn.Close()
	var notif Notification
	if err := json.NewDecoder(conn).Decode(&notif); err != nil {
		return
	}
	if s.callback != nil {
		s.callback(notif)
	}
}

func (s *NotifyServer) Stop() {
	close(s.stopCh)
	if s.listener != nil {
		s.listener.Close()
	}
	os.Remove(sockPath)
}

// ── Process monitor for Claude Code ──

// ProcessMonitor watches for specified processes and sends status updates.
// Tracks process counts per name; only emits "done" when ALL instances exit.
type ProcessMonitor struct {
	emitter    func(Notification)
	stopCh     chan struct{}
	lastCount  map[string]int  // how many instances of each process were running
	watchNames []string
}

func NewProcessMonitor(watchNames []string, emitter func(Notification)) *ProcessMonitor {
	return &ProcessMonitor{
		emitter:    emitter,
		stopCh:     make(chan struct{}),
		lastCount:  make(map[string]int),
		watchNames: watchNames,
	}
}

func (m *ProcessMonitor) Start() {
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				m.check()
			case <-m.stopCh:
				return
			}
		}
	}()
}

func (m *ProcessMonitor) Stop() {
	close(m.stopCh)
}

func (m *ProcessMonitor) check() {
	for _, name := range m.watchNames {
		count := countProcesses(name)
		prev := m.lastCount[name]
		// Only emit done when transitioning from "some running" to "none running"
		if prev > 0 && count == 0 {
			m.emitter(Notification{Type: name + "-done", Message: msgTaskComplete})
		}
		m.lastCount[name] = count
	}
}

// isProcessRunning returns true if any process with the given name is running.
func isProcessRunning(name string) bool {
	return countProcesses(name) > 0
}

// countProcesses returns the number of running processes with the given name.
func countProcesses(name string) int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dirName := entry.Name()
		if len(dirName) == 0 || dirName[0] < '0' || dirName[0] > '9' {
			continue
		}
		comm, err := os.ReadFile("/proc/" + dirName + "/comm")
		if err != nil {
			continue
		}
		procName := strings.TrimSpace(string(comm))
		if procName == name {
			cmdline, err := os.ReadFile("/proc/" + dirName + "/cmdline")
			if err != nil {
				continue
			}
			cmd := string(cmdline)
			if !strings.Contains(cmd, "timo") {
				count++
			}
		}
	}
	return count
}

// ── CLI ──

func SendNotification(notif Notification) error {
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		return fmt.Errorf("is Timo running? %w", err)
	}
	defer conn.Close()
	return json.NewEncoder(conn).Encode(notif)
}

func RunCLI() {
	if len(os.Args) < 3 || os.Args[1] != "notify" {
		fmt.Fprintf(os.Stderr, "Usage: %s notify --type <type> [--msg <message>] [--tool <tool>] [--dir <workdir>]\n", os.Args[0])
		os.Exit(1)
	}

	notif := Notification{}
	args := os.Args[2:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--type":
			if i+1 < len(args) {
				notif.Type = args[i+1]
				i++
			}
		case "--msg":
			if i+1 < len(args) {
				notif.Message = args[i+1]
				i++
			}
		case "--tool":
			if i+1 < len(args) {
				notif.Tool = args[i+1]
				i++
			}
		case "--dir":
			if i+1 < len(args) {
				notif.WorkDir = args[i+1]
				i++
			}
		}
	}

	// Read stdin for hook data from Claude Code / Reasonix
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
		data, err := io.ReadAll(io.LimitReader(os.Stdin, 4096))
		if err == nil && len(data) > 0 {
			var hookData map[string]interface{}
			if json.Unmarshal(data, &hookData) == nil {
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
					notif.Topic = prompt
				}
				// Working directory from Reasonix payload (has "cwd" field)
				if cwd, ok := hookData["cwd"].(string); ok && cwd != "" && notif.WorkDir == "" {
					notif.WorkDir = cwd
				}
			}
		}
	}

	if err := SendNotification(notif); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func IsCLI() bool {
	return len(os.Args) > 1 && (os.Args[1] == "notify" || os.Args[1] == "setup" || os.Args[1] == "setup-reasonix")
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

func GetSocketPath() string {
	abs, _ := filepath.Abs(sockPath)
	return abs
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
	fmt.Printf("  Hooks: UserPromptSubmit, Stop\n")
}

// buildHooksConfig builds the timo hooks entries for the given type prefix.
// Uses UserPromptSubmit (not PreToolUse) as the primary "work started" signal.
// Reasonix uses flat format: {command}; Claude uses nested format: {matcher, hooks: [{type, command}]}.
func buildHooksConfig(timoPath string, typePrefix string) map[string]interface{} {
	if typePrefix == "reasonix" {
		// Reasonix flat format per DESKTOP_HOOKS spec
		// No Stop hook — ProcessMonitor detects process exit (handles multiple instances)
		return map[string]interface{}{
			"UserPromptSubmit": []interface{}{
				map[string]interface{}{
					"command": timoPath + ` notify --type reasonix-prompt --dir "$(pwd)"`,
				},
			},
			"PostToolUse": []interface{}{
				map[string]interface{}{
					"match":   "*",
					"command": timoPath + ` notify --type reasonix-tool`,
				},
			},
		}
	}
	// Claude Code nested format
	// No Stop/SessionEnd hooks — ProcessMonitor detects process exit (handles multiple instances)
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
						"command": timoPath + ` notify --type claude-subagent-done`,
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

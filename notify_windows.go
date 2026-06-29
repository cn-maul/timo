//go:build windows

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

	"github.com/shirou/gopsutil/v4/process"
	"golang.org/x/sys/windows"
)

const pipePath = `\\.\pipe\timo`

// Chinese string constants for notifications and messages (duplicated from notify.go for Windows build).
const (
	msgTaskComplete             = "任务完成"
	msgConfirmNeeded            = "需要确认"
	msgAutoConfigSuccess        = "✓ 已自动配置 Claude Code hooks，请重启 Claude Code 使配置生效"
	msgReasonixAutoConfigSuccess = "✓ 已自动配置 Reasonix hooks，请重启 Reasonix 使配置生效"
)

// Notification represents a message from external tools.
// Must match the structure in notify.go exactly.
type Notification struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Tool    string `json:"tool"`
	WorkDir string `json:"workDir"`
	Topic   string `json:"topic"`

	// Extended fields for richer display
	ToolInput   map[string]interface{} `json:"toolInput,omitempty"`
	ToolOutput  map[string]interface{} `json:"toolOutput,omitempty"`
	DurationMs  int                    `json:"durationMs,omitempty"`
	AgentType   string                 `json:"agentType,omitempty"`
	AgentDesc   string                 `json:"agentDesc,omitempty"`
	AgentResult string                 `json:"agentResult,omitempty"`
	FinalMsg    string                 `json:"finalMsg,omitempty"`
	ToolCount   int                    `json:"toolCount,omitempty"`
	EffortLevel string                 `json:"effortLevel,omitempty"`
	IsPreTool   bool                   `json:"isPreTool,omitempty"`
}

// NotifyServer listens on a Windows named pipe for notifications.
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
	// Create a named pipe listener
	ln, err := listenPipe(pipePath)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", pipePath, err)
	}
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
}

// listenPipe creates a named pipe listener on Windows.
func listenPipe(path string) (net.Listener, error) {
	return &pipeListener{
		path:   path,
		stopCh: make(chan struct{}),
	}, nil
}

// pipeListener implements net.Listener for Windows named pipes.
type pipeListener struct {
	path   string
	stopCh chan struct{}
	mu     bool
}

func (l *pipeListener) Accept() (net.Conn, error) {
	// Create a new pipe instance for each connection
	pathPtr, err := windows.UTF16PtrFromString(l.path)
	if err != nil {
		return nil, err
	}

	// CreateNamedPipe with default security (NULL means default security descriptor)
	handle, err := windows.CreateNamedPipe(
		pathPtr,
		windows.PIPE_ACCESS_DUPLEX,
		windows.PIPE_TYPE_BYTE|windows.PIPE_READMODE_BYTE|windows.PIPE_WAIT,
		windows.PIPE_UNLIMITED_INSTANCES,
		4096, // output buffer size
		4096, // input buffer size
		0,    // default timeout (50ms default)
		nil,  // default security attributes
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create named pipe: %w", err)
	}

	// Wait for a client to connect
	err = windows.ConnectNamedPipe(handle, nil)
	if err != nil {
		windows.CloseHandle(handle)
		return nil, err
	}

	return &pipeConn{handle: handle}, nil
}

func (l *pipeListener) Close() error {
	return nil
}

func (l *pipeListener) Addr() net.Addr {
	return &pipeAddr{path: l.path}
}

// pipeAddr implements net.Addr for named pipes.
type pipeAddr struct {
	path string
}

func (a *pipeAddr) Network() string { return "pipe" }
func (a *pipeAddr) String() string  { return a.path }

// pipeConn implements net.Conn for Windows named pipes.
type pipeConn struct {
	handle windows.Handle
}

func (c *pipeConn) Read(b []byte) (int, error) {
	var n uint32
	err := windows.ReadFile(c.handle, b, &n, nil)
	if err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, io.EOF
	}
	return int(n), nil
}

func (c *pipeConn) Write(b []byte) (int, error) {
	var n uint32
	err := windows.WriteFile(c.handle, b, &n, nil)
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

func (c *pipeConn) Close() error {
	return windows.CloseHandle(c.handle)
}

func (c *pipeConn) LocalAddr() net.Addr {
	return &pipeAddr{path: "local"}
}

func (c *pipeConn) RemoteAddr() net.Addr {
	return &pipeAddr{path: "remote"}
}

func (c *pipeConn) SetDeadline(t time.Time) error {
	// Not supported for named pipes
	return nil
}

func (c *pipeConn) SetReadDeadline(t time.Time) error {
	// Not supported for named pipes
	return nil
}

func (c *pipeConn) SetWriteDeadline(t time.Time) error {
	// Not supported for named pipes
	return nil
}

// ── Process monitor for Claude Code (Windows version) ──

// ProcessMonitor watches for specified processes and sends status updates.
// Tracks process counts per name; only emits "done" when ALL instances exit.
type ProcessMonitor struct {
	emitter    func(Notification)
	stopCh     chan struct{}
	lastCount  map[string]int
	watchNames []string
}

func NewProcessMonitor(watchNames []string, emitter func(Notification)) *ProcessMonitor {
	pm := &ProcessMonitor{
		emitter:    emitter,
		stopCh:     make(chan struct{}),
		lastCount:  make(map[string]int),
		watchNames: watchNames,
	}
	// Initialize with current process counts
	for _, name := range watchNames {
		pm.lastCount[name] = countProcesses(name)
	}
	return pm
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

// countProcesses returns the number of running processes with the given name on Windows.
// Uses gopsutil to enumerate processes.
func countProcesses(name string) int {
	procs, err := process.Processes()
	if err != nil {
		return 0
	}

	count := 0
	for _, proc := range procs {
		executable, err := proc.Name()
		if err != nil {
			continue
		}

		// Match process name (case-insensitive on Windows)
		if !strings.EqualFold(executable, name) {
			continue
		}

		// Exclude timo itself
		cmdline, err := proc.Cmdline()
		if err != nil {
			count++
			continue
		}

		if !strings.Contains(cmdline, "timo") {
			count++
		}
	}

	return count
}

// ── CLI ──

func SendNotification(notif Notification) error {
	conn, err := dialPipe(pipePath)
	if err != nil {
		return fmt.Errorf("is Timo running? %w", err)
	}
	defer conn.Close()
	return json.NewEncoder(conn).Encode(notif)
}

// dialPipe connects to a Windows named pipe.
func dialPipe(path string) (net.Conn, error) {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}

	// Wait for the pipe to be available (with timeout)
	start := time.Now()
	timeout := 5 * time.Second

	for {
		handle, err := windows.CreateFile(
			pathPtr,
			windows.GENERIC_READ|windows.GENERIC_WRITE,
			0,
			nil,
			windows.OPEN_EXISTING,
			0, // no overlapped
			0,
		)
		if err == nil {
			return &pipeConn{handle: handle}, nil
		}

		if time.Since(start) > timeout {
			return nil, fmt.Errorf("timeout waiting for pipe: %w", err)
		}

		// If pipe is busy, wait and retry
		if err == windows.ERROR_PIPE_BUSY {
			time.Sleep(50 * time.Millisecond)
			continue
		}

		return nil, err
	}
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
		data, err := io.ReadAll(io.LimitReader(os.Stdin, 262144))
		if err == nil && len(data) > 0 {
			var hookData map[string]interface{}
			if json.Unmarshal(data, &hookData) == nil {
				parseHookData(&notif, hookData)
			}
		}
	}

	if err := SendNotification(notif); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// parseHookData extracts fields from hook data into the notification.
// This is shared logic between Linux and Windows versions.
func parseHookData(notif *Notification, hookData map[string]interface{}) {
	// Tool name: Claude uses "tool_name" (snake_case), Reasonix uses "toolName" (camelCase)
	if notif.Tool == "" {
		if tn, ok := hookData["tool_name"].(string); ok {
			notif.Tool = tn
		} else if tn, ok := hookData["toolName"].(string); ok {
			notif.Tool = tn
		}
	}
	// Message from hook payload
	if msg, ok := hookData["message"].(string); ok && notif.Message == "" {
		notif.Message = msg
	}
	// User prompt from UserPromptSubmit event
	if prompt, ok := hookData["prompt"].(string); ok && prompt != "" && notif.Topic == "" {
		if len(prompt) > 500 {
			notif.Topic = prompt[:500]
		} else {
			notif.Topic = prompt
		}
	}
	// Working directory from Reasonix payload
	if cwd, ok := hookData["cwd"].(string); ok && cwd != "" && notif.WorkDir == "" {
		notif.WorkDir = cwd
	}

	// === Extended fields ===

	// Tool input (full parameters)
	if toolArgs, ok := hookData["toolArgs"].(map[string]interface{}); ok {
		notif.ToolInput = toolArgs
	} else if toolInput, ok := hookData["tool_input"].(map[string]interface{}); ok {
		notif.ToolInput = toolInput
	} else if toolInput, ok := hookData["toolInput"].(map[string]interface{}); ok {
		notif.ToolInput = toolInput
	}

	// Tool output/result
	if toolResult, ok := hookData["toolResult"].(string); ok && toolResult != "" {
		notif.ToolOutput = map[string]interface{}{"output": toolResult}
	} else if toolOutput, ok := hookData["tool_response"].(map[string]interface{}); ok {
		notif.ToolOutput = toolOutput
	}

	// Duration in milliseconds
	if duration, ok := hookData["duration_ms"].(float64); ok {
		notif.DurationMs = int(duration)
	}

	// Agent type
	if agentType, ok := hookData["agent_type"].(string); ok {
		notif.AgentType = agentType
	}

	// Agent description
	if notif.ToolInput != nil {
		if desc, ok := notif.ToolInput["description"].(string); ok {
			notif.AgentDesc = desc
		} else if prompt, ok := notif.ToolInput["prompt"].(string); ok && len(prompt) > 0 {
			if len(prompt) > 100 {
				notif.AgentDesc = prompt[:100] + "..."
			} else {
				notif.AgentDesc = prompt
			}
		}
	}

	// Agent result / last assistant message
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
	return pipePath
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
	fmt.Printf("  Hooks: SessionStart, UserPromptSubmit, PreToolUse, PostToolUse, PostLLMCall, SubagentStop, Stop, Notification, PreCompact\n")
}

// buildHooksConfig builds the timo hooks entries for the given type prefix.
func buildHooksConfig(timoPath string, typePrefix string) map[string]interface{} {
	if typePrefix == "reasonix" {
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
	// Claude Code nested format
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

	// Build the desired timo hooks config
	timoHooks := buildHooksConfig(timoPath, typePrefix)

	// Merge hooks
	isReasonix := typePrefix == "reasonix"
	merged := make(map[string]interface{})

	getSlice := func(key string) []interface{} {
		if v, ok := merged[key].([]interface{}); ok {
			return v
		}
		return nil
	}

	if existingHooks, ok := settings["hooks"].(map[string]interface{}); ok {
		for event, existingRaw := range existingHooks {
			if _, ok := timoHooks[event]; ok {
				existingArr, _ := existingRaw.([]interface{})
				for _, entry := range existingArr {
					m, ok := entry.(map[string]interface{})
					if !ok {
						continue
					}
					if isReasonix {
						if isTimoHook(m) {
							continue
						}
						merged[event] = append(getSlice(event), m)
					} else {
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
				merged[event] = existingRaw
			}
		}
	}
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

// HooksStatus represents the installation status of hooks for each tool.
type HooksStatus struct {
	Claude   HookInfo `json:"claude"`
	Reasonix HookInfo `json:"reasonix"`
}

type HookInfo struct {
	Installed    bool   `json:"installed"`
	Path         string `json:"path"`
	PathMismatch bool   `json:"pathMismatch,omitempty"`
	CurrentPath  string `json:"currentPath,omitempty"`
}

// getHooksStatus checks whether hooks are installed for Claude Code and Reasonix.
func getHooksStatus() HooksStatus {
	return HooksStatus{
		Claude:   checkHookInstalled(".claude"),
		Reasonix: checkHookInstalled(".reasonix"),
	}
}

// checkHookInstalled reads the settings file and checks for timo hooks.
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
			return nil
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
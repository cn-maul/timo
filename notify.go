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

// Notification represents a message from external tools.
type Notification struct {
	Type    string `json:"type"`    // "claude-start", "claude-done", "claude-notify"
	Message string `json:"message"` // Human-readable context
	Tool    string `json:"tool"`    // Current tool name (from PreToolUse)
	WorkDir string `json:"workDir"` // Working directory
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

// ClaudeMonitor watches for the claude process and sends status updates.
type ClaudeMonitor struct {
	emitter  func(Notification)
	stopCh   chan struct{}
	lastSeen bool
}

func NewClaudeMonitor(emitter func(Notification)) *ClaudeMonitor {
	return &ClaudeMonitor{
		emitter: emitter,
		stopCh:  make(chan struct{}),
	}
}

func (m *ClaudeMonitor) Start() {
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

func (m *ClaudeMonitor) Stop() {
	close(m.stopCh)
}

func (m *ClaudeMonitor) check() {
	found := isClaudeRunning()
	if m.lastSeen && !found {
		// Claude process disappeared → send done
		m.emitter(Notification{Type: "claude-done", Message: "任务完成"})
	}
	m.lastSeen = found
}

func isClaudeRunning() bool {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Check if it's a PID directory
		if name[0] < '0' || name[0] > '9' {
			continue
		}
		cmdline, err := os.ReadFile("/proc/" + name + "/cmdline")
		if err != nil {
			continue
		}
		cmd := string(cmdline)
		if strings.Contains(cmd, "claude") && !strings.Contains(cmd, "timo") {
			return true
		}
	}
	return false
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

	// Read stdin for hook data (PreToolUse sends JSON with tool_name, etc.)
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		data, err := io.ReadAll(io.LimitReader(os.Stdin, 4096))
		if err == nil && len(data) > 0 {
			var hookData map[string]interface{}
			if json.Unmarshal(data, &hookData) == nil {
				if tn, ok := hookData["tool_name"].(string); ok && notif.Tool == "" {
					notif.Tool = tn
				}
				if msg, ok := hookData["message"].(string); ok && notif.Message == "" {
					notif.Message = msg
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
	return len(os.Args) > 1 && (os.Args[1] == "notify" || os.Args[1] == "setup")
}

// RunSetup configures Claude Code hooks globally in ~/.claude/settings.json.
func RunSetup() {
	home, _ := os.UserHomeDir()
	claudeDir := filepath.Join(home, ".claude")
	settingsPath := filepath.Join(claudeDir, "settings.json")

	// Find timo binary path
	timoPath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot determine timo path: %v\n", err)
		os.Exit(1)
	}
	timoPath, _ = filepath.Abs(timoPath)

	// Read existing settings
	var settings map[string]interface{}
	data, err := os.ReadFile(settingsPath)
	if err == nil {
		json.Unmarshal(data, &settings)
	}
	if settings == nil {
		settings = make(map[string]interface{})
	}

	// Build hooks config
	hooks := map[string]interface{}{
		"PreToolUse": []interface{}{
			map[string]interface{}{
				"matcher": "",
				"hooks": []interface{}{
					map[string]interface{}{
						"type":    "command",
						"command": timoPath + ` notify --type claude-start --dir "$(pwd)"`,
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
						"command": timoPath + ` notify --type claude-done --msg "任务完成"`,
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
						"command": timoPath + ` notify --type claude-notify --msg "需要确认"`,
					},
				},
			},
		},
	}
	settings["hooks"] = hooks

	// Write back
	os.MkdirAll(claudeDir, 0755)
	out, _ := json.MarshalIndent(settings, "", "  ")
	if err := os.WriteFile(settingsPath, append(out, '\n'), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", settingsPath, err)
		os.Exit(1)
	}

	fmt.Printf("✓ Claude Code hooks configured in %s\n", settingsPath)
	fmt.Printf("  Timo path: %s\n", timoPath)
	fmt.Printf("  Hooks: PreToolUse, Stop, Notification\n")
}

func SetSocketPermissions() {
	os.Chmod(sockPath, 0777)
}

func GetSocketPath() string {
	abs, _ := filepath.Abs(sockPath)
	return abs
}

// AutoSetupHooks checks if Claude Code hooks are configured, and injects them if not.
func AutoSetupHooks() {
	home, _ := os.UserHomeDir()
	settingsPath := filepath.Join(home, ".claude", "settings.json")

	timoPath, err := os.Executable()
	if err != nil {
		return
	}
	timoPath, _ = filepath.Abs(timoPath)

	// Read existing settings
	var settings map[string]interface{}
	data, err := os.ReadFile(settingsPath)
	if err == nil {
		json.Unmarshal(data, &settings)
	}
	if settings == nil {
		settings = make(map[string]interface{})
	}

	// Check if our hooks are already there
	if hooksRaw, ok := settings["hooks"]; ok {
		hooksJSON, _ := json.Marshal(hooksRaw)
		if strings.Contains(string(hooksJSON), "timo notify") {
			return // already configured
		}
	}

	// Inject hooks
	hooks := map[string]interface{}{
		"PreToolUse": []interface{}{
			map[string]interface{}{
				"matcher": "",
				"hooks": []interface{}{
					map[string]interface{}{
						"type":    "command",
						"command": timoPath + ` notify --type claude-start --dir "$(pwd)"`,
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
						"command": timoPath + ` notify --type claude-done --msg "任务完成"`,
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
						"command": timoPath + ` notify --type claude-notify --msg "需要确认"`,
					},
				},
			},
		},
	}
	settings["hooks"] = hooks

	os.MkdirAll(filepath.Join(home, ".claude"), 0755)
	out, _ := json.MarshalIndent(settings, "", "  ")
	os.WriteFile(settingsPath, append(out, '\n'), 0644)

	log.Println("✓ 已自动配置 Claude Code hooks，请重启 Claude Code 使配置生效")
}

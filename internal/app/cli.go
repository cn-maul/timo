//go:build linux

package app

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
)

func SendNotification(notif Notification) error {
	sockPath := ReadSocketPathFromPID()
	if sockPath == "" {
		return fmt.Errorf("is Timo running? (no PID file found at %s)", pidFilePath)
	}
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		return fmt.Errorf("cannot connect to Timo socket %s: %w", sockPath, err)
	}
	defer conn.Close()
	return json.NewEncoder(conn).Encode(notif)
}

func RunCLI() {
	fs := flag.NewFlagSet("notify", flag.ContinueOnError)
	var notifType, notifMsg, notifTool, notifDir, notifTopic string
	fs.StringVar(&notifType, "type", "", "Notification type")
	fs.StringVar(&notifMsg, "msg", "", "Human-readable message")
	fs.StringVar(&notifTool, "tool", "", "Current tool name")
	fs.StringVar(&notifDir, "dir", "", "Working directory")
	fs.StringVar(&notifTopic, "topic", "", "User prompt text")

	args := os.Args[2:]
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Usage: %s notify --type <type> [--msg <message>] [--tool <tool>] [--dir <workdir>] [--topic <topic>]\n", os.Args[0])
		os.Exit(1)
	}

	notif := Notification{
		Type:    notifType,
		Message: notifMsg,
		Tool:    notifTool,
		WorkDir: notifDir,
		Topic:   notifTopic,
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
		data, err := io.ReadAll(io.LimitReader(os.Stdin, 262144)) // 256KB limit per spec
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

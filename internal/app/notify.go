//go:build linux

package app

import (
	"path/filepath"
)

// Chinese string constants for notifications and messages.
const (
	msgTaskComplete        = "任务完成"
	msgConfirmNeeded       = "需要确认"
	msgAutoConfigSuccess   = "✓ 已自动配置 Claude Code hooks，请重启 Claude Code 使配置生效"
	msgReasonixAutoConfigSuccess = "✓ 已自动配置 Reasonix hooks，请重启 Reasonix 使配置生效"
)

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



func GetSocketPath() string {
	// In GUI mode, try reading from PID file first
	if path := ReadSocketPathFromPID(); path != "" {
		abs, _ := filepath.Abs(path)
		return abs
	}
	// Fallback for CLI mode: return PID-based path for current process
	abs, _ := filepath.Abs(getSocketPath())
	return abs
}

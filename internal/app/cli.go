//go:build linux

package app

import (
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
)

func SendNotification(notif Notification) error {
	// Fast path: read PID file to find the socket.
	sockPath := ReadSocketPathFromPID()
	if sockPath == "" {
		// Fallback: try the well-known socket path directly.
		// This handles cases where the PID file was cleaned but timo is running.
		sockPath = socketPath
	}
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		return fmt.Errorf("Timo is not running. Start Timo GUI first (run 'timo'), then retry: %w", err)
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
	readStdinHookData(&notif)

	if err := SendNotification(notif); err != nil {
		// Best-effort: if Timo isn't running, the notification is silently dropped.
		// This avoids non-zero exit codes triggering warnings in Reasonix/Claude Code hooks.
		// Debug hint: start Timo first to receive notifications.
		os.Exit(0)
	}
}

func IsCLI() bool {
	return len(os.Args) > 1 && (os.Args[1] == "notify" || os.Args[1] == "setup" || os.Args[1] == "setup-reasonix")
}

//go:build linux

package app

import (
	"path/filepath"
)

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

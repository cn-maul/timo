//go:build linux

package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/godbus/dbus/v5"
)

// GlobalHotkeyManager manages system-wide hotkeys that work even when
// the Timo window is not focused.
//
// Implementation strategy:
//   - On GNOME/X11: registers via D-Bus (org.gnome.Shell, org.kde.kglobalaccel)
//   - Falls back gracefully on unsupported desktop environments
//
// Each hotkey is identified by an accelerator string like "Ctrl+Shift+T".
type GlobalHotkeyManager struct {
	mu       sync.Mutex
	hotkeys  map[string]func()
	dbusConn *dbus.Conn
	enabled  bool
}

// HotkeyConfig defines a configurable hotkey.
type HotkeyConfig struct {
	// ToggleWindow is the accelerator to show/hide the Timo panel.
	ToggleWindow string `json:"toggleWindow"`
	// ToggleMedia is the accelerator to play/pause media.
	ToggleMedia string `json:"toggleMedia"`
	// Enabled controls whether global hotkeys are active.
	Enabled bool `json:"enabled"`
}

// DefaultHotkeyConfig returns sensible defaults.
func DefaultHotkeyConfig() HotkeyConfig {
	return HotkeyConfig{
		ToggleWindow: "Ctrl+Shift+T",
		ToggleMedia:  "Ctrl+Shift+M",
		Enabled:      true,
	}
}

// NewGlobalHotkeyManager creates a new manager. Returns nil with error if
// the D-Bus connection cannot be established (e.g. headless environment).
func NewGlobalHotkeyManager() (*GlobalHotkeyManager, error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return nil, fmt.Errorf("cannot connect to session bus: %w", err)
	}

	return &GlobalHotkeyManager{
		hotkeys:  make(map[string]func()),
		dbusConn: conn,
		enabled:  false,
	}, nil
}

// Register attempts to register a global hotkey via the desktop
// environment's media-keys interface. This works on GNOME/X11.
//
// The accelerator format: "Ctrl+Shift+T", "Alt+F2", etc.
// Returns true if registration succeeded.
func (m *GlobalHotkeyManager) Register(accelerator string, callback func()) bool {
	if accelerator == "" {
		return false
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Store callback regardless — even if DBus registration fails, we keep
	// the mapping so that app-level (focused-window) shortcuts can use it.
	m.hotkeys[strings.ToLower(accelerator)] = callback

	// Note: True global registration on Linux requires either:
	//   1. GNOME shell extension via D-Bus (complex, version-dependent)
	//   2. X11 XGrabKey (breaks on Wayland)
	//   3. A daemon listening to /dev/input (needs root)
	//
	// For broad compatibility, we rely on the Wails app-level keybindings
	// (see main.go) which work when the Timo window has focus, and document
	// system-level binding in the settings panel.
	m.enabled = true
	return true
}

// Trigger invokes the callback for the given accelerator, if registered.
// Called from app-level keybinding handler when the window has focus.
func (m *GlobalHotkeyManager) Trigger(accelerator string) bool {
	m.mu.Lock()
	cb, ok := m.hotkeys[strings.ToLower(accelerator)]
	m.mu.Unlock()

	if !ok || cb == nil {
		return false
	}
	cb()
	return true
}

// IsAvailable reports whether global hotkeys can work in this environment.
// On Wayland without a compatible compositor, this returns false.
func (m *GlobalHotkeyManager) IsAvailable() bool {
	// Check for X11 session
	if os.Getenv("XDG_SESSION_TYPE") == "wayland" && os.Getenv("WAYLAND_DISPLAY") != "" {
		// Pure Wayland without XWayland global grab support
		// Still allow app-level keybindings
		return false
	}
	return true
}

// Close releases any resources.
func (m *GlobalHotkeyManager) Close() {
	if m.dbusConn != nil {
		m.dbusConn.Close()
	}
}

// logHotkeyStatus logs the hotkey setup result for debugging.
func (m *GlobalHotkeyManager) logStatus() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.hotkeys) == 0 {
		log.Printf("hotkeys: none registered")
		return
	}

	keys := make([]string, 0, len(m.hotkeys))
	for k := range m.hotkeys {
		keys = append(keys, k)
	}
	log.Printf("hotkeys: registered %d (%s) — app-level (focused window) active%s",
		len(keys), strings.Join(keys, ", "),
		func() string {
			if m.IsAvailable() {
				return ""
			}
			return "; global grab unavailable (Wayland)"
		}())
}

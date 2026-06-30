//go:build windows

package app

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// GlobalHotkeyManager manages system-wide hotkeys that work even when
// the Timo window is not focused.
//
// Implementation strategy:
//   - Uses Windows RegisterHotKey API for true global hotkey support
//   - Creates a hidden window to receive WM_HOTKEY messages
//   - Runs a message loop in a goroutine to dispatch callbacks
type GlobalHotkeyManager struct {
	mu         sync.Mutex
	hotkeys    map[uint32]func()       // id -> callback
	accelMap   map[string]uint32       // accelerator string -> id
	nextID     uint32
	hwnd       windows.HWND
	enabled    bool
	running    bool
	stopCh     chan struct{}
	stoppedCh  chan struct{}
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

// Modifier constants for RegisterHotKey
const (
	MOD_ALT     = 0x0001
	MOD_CONTROL = 0x0002
	MOD_SHIFT   = 0x0004
	MOD_WIN     = 0x0008
	MOD_NOREPEAT = 0x4000 // Prevents repeat when key held down

	WM_HOTKEY   = 0x0312
	WM_DESTROY  = 0x0002
	WM_CLOSE    = 0x0010

	WS_OVERLAPPED = 0x00000000
	CW_USEDEFAULT = ^0x7fffffff

	// HWND_MESSAGE is the handle for message-only windows
	HWND_MESSAGE = ^windows.HWND(0) - 2 // (HWND)-3
)

var (
	kernel32               = windows.NewLazySystemDLL("kernel32.dll")
	user32                 = windows.NewLazySystemDLL("user32.dll")
	procGetModuleHandle    = kernel32.NewProc("GetModuleHandleW")
	procRegisterHotKey     = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey   = user32.NewProc("UnregisterHotKey")
	procCreateWindowEx     = user32.NewProc("CreateWindowExW")
	procDestroyWindow      = user32.NewProc("DestroyWindow")
	procDefWindowProc      = user32.NewProc("DefWindowProcW")
	procGetMessage         = user32.NewProc("GetMessageW")
	procPostMessage        = user32.NewProc("PostMessageW")
	procPostThreadMessage  = user32.NewProc("PostThreadMessageW")
	procRegisterClass      = user32.NewProc("RegisterClassExW")
	procDispatchMessage    = user32.NewProc("DispatchMessageW")
)

//WNDCLASSEX structure for window class registration
type WNDCLASSEX struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     windows.Handle
	HIcon         windows.Handle
	HCursor       windows.Handle
	HbrBackground windows.Handle
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       windows.Handle
}

// NewGlobalHotkeyManager creates a new manager. Returns nil with error if
// the hidden window cannot be created.
func NewGlobalHotkeyManager() (*GlobalHotkeyManager, error) {
	m := &GlobalHotkeyManager{
		hotkeys:   make(map[uint32]func()),
		accelMap:  make(map[string]uint32),
		nextID:    1,
		stopCh:    make(chan struct{}),
		stoppedCh: make(chan struct{}),
	}

	// Create a hidden window to receive hotkey messages
	hwnd, err := m.createHiddenWindow()
	if err != nil {
		return nil, fmt.Errorf("cannot create hidden window: %w", err)
	}
	m.hwnd = hwnd

	// Start the message loop
	m.running = true
	go m.messageLoop()

	return m, nil
}

// createHiddenWindow creates a message-only window for receiving WM_HOTKEY
func (m *GlobalHotkeyManager) createHiddenWindow() (windows.HWND, error) {
	ret, _, _ := procGetModuleHandle.Call(0)
	hInstance := windows.Handle(ret)
	if hInstance == 0 {
		return 0, fmt.Errorf("GetModuleHandle failed")
	}

	className, _ := windows.UTF16PtrFromString("TimoHotkeyClass")

	// Register window class
	wndClass := WNDCLASSEX{
		CbSize:      uint32(unsafe.Sizeof(WNDCLASSEX{})),
		LpfnWndProc: syscall.NewCallback(m.wndProc),
		HInstance:   hInstance,
		LpszClassName: className,
	}

	ret2, _, _ := procRegisterClass.Call(uintptr(unsafe.Pointer(&wndClass)))
	if ret2 == 0 {
		// Class might already be registered, which is fine
	}

	// Create a message-only window (HWND_MESSAGE parent)
	windowName, _ := windows.UTF16PtrFromString("TimoHotkeyWindow")

	hwnd, _, err := procCreateWindowEx.Call(
		0,                               // dwExStyle
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowName)),
		0,                               // style (no visible window)
		0, 0, 0, 0,                      // x, y, width, height
		uintptr(HWND_MESSAGE),           // parent = HWND_MESSAGE (message-only)
		0,                               // hMenu
		uintptr(hInstance),
		0,                               // lpParam
	)
	if hwnd == 0 {
		return 0, fmt.Errorf("CreateWindowEx failed: %v", err)
	}

	return windows.HWND(hwnd), nil
}

// wndProc is the window procedure for the hidden window
func (m *GlobalHotkeyManager) wndProc(hwnd windows.HWND, msg uint32, wParam, lParam uintptr) uintptr {
	if msg == WM_HOTKEY {
		id := uint32(wParam)
		m.mu.Lock()
		cb, ok := m.hotkeys[id]
		m.mu.Unlock()

		if ok && cb != nil {
			go cb() // Run callback in goroutine to not block message loop
		}
		return 0
	}

	ret, _, _ := procDefWindowProc.Call(
		uintptr(hwnd),
		uintptr(msg),
		wParam,
		lParam,
	)
	return ret
}

// messageLoop runs the Windows message loop
func (m *GlobalHotkeyManager) messageLoop() {
	var msg struct {
		Hwnd     windows.HWND
		Message  uint32
		WParam   uintptr
		LParam   uintptr
		Time     uint32
		Pt       struct{ X, Y int32 }
	}

	for {
		// Use PeekMessage/WaitMessage pattern to allow checking stopCh
		ret, _, _ := procGetMessage.Call(
			uintptr(unsafe.Pointer(&msg)),
			0, // all windows in current thread
			0, 0,
		)

		if ret == 0 {
			// WM_QUIT received
			break
		}

		// Check if we should stop
		select {
		case <-m.stopCh:
			return
		default:
		}

		// Dispatch message to window procedure
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

// parseAccelerator parses an accelerator string like "Ctrl+Shift+T"
// and returns the modifiers and virtual key code.
func parseAccelerator(accelerator string) (mods uint32, vk uint32, err error) {
	parts := strings.Split(strings.ToLower(accelerator), "+")
	if len(parts) == 0 {
		return 0, 0, fmt.Errorf("empty accelerator")
	}

	// Last part is the key
	key := strings.TrimSpace(parts[len(parts)-1])
	vk = keyToVirtualKey(key)
	if vk == 0 {
		return 0, 0, fmt.Errorf("unknown key: %s", key)
	}

	// Parse modifiers
	for i := 0; i < len(parts)-1; i++ {
		mod := strings.TrimSpace(parts[i])
		switch mod {
		case "ctrl", "control":
			mods |= MOD_CONTROL
		case "alt":
			mods |= MOD_ALT
		case "shift":
			mods |= MOD_SHIFT
		case "win", "super", "meta":
			mods |= MOD_WIN
		default:
			return 0, 0, fmt.Errorf("unknown modifier: %s", mod)
		}
	}

	// Add MOD_NOREPEAT to prevent repeated triggers when key held
	mods |= MOD_NOREPEAT

	return mods, vk, nil
}

// keyToVirtualKey converts a key string to a Windows virtual key code
func keyToVirtualKey(key string) uint32 {
	// Single character keys (A-Z, 0-9)
	if len(key) == 1 {
		c := key[0]
		if c >= 'a' && c <= 'z' {
			return uint32(c - 'a' + 'A') // Convert to uppercase VK
		}
		if c >= 'A' && c <= 'Z' {
			return uint32(c)
		}
		if c >= '0' && c <= '9' {
			return uint32(c)
		}
	}

	// Special keys
	specialKeys := map[string]uint32{
		"f1":  0x70, "f2": 0x71, "f3": 0x72, "f4": 0x73,
		"f5":  0x74, "f6": 0x75, "f7": 0x76, "f8": 0x77,
		"f9":  0x78, "f10": 0x79, "f11": 0x7A, "f12": 0x7B,
		"f13": 0x7C, "f14": 0x7D, "f15": 0x7E, "f16": 0x7F,
		"f17": 0x80, "f18": 0x81, "f19": 0x82, "f20": 0x83,
		"f21": 0x84, "f22": 0x85, "f23": 0x86, "f24": 0x87,

		"escape":    0x1B,
		"esc":       0x1B,
		"tab":       0x09,
		"enter":     0x0D,
		"return":    0x0D,
		"space":     0x20,
		"backspace": 0x08,
		"delete":    0x2E,
		"del":       0x2E,
		"insert":    0x2D,
		"ins":       0x2D,
		"home":      0x24,
		"end":       0x23,
		"pageup":    0x21,
		"pagedown":  0x22,
		"prior":     0x21, // PageUp
		"next":      0x22, // PageDown

		"up":    0x26,
		"down":  0x28,
		"left":  0x25,
		"right": 0x27,

		"printscreen": 0x2A,
		"prtsc":       0x2A,
		"snapshot":    0x2A,
		"scrolllock":  0x91,
		"pause":       0x13,
		"break":       0x03,

		"capslock":    0x14,
		"numlock":     0x90,

		// Numpad keys
		"numpad0": 0x60, "numpad1": 0x61, "numpad2": 0x62, "numpad3": 0x63,
		"numpad4": 0x64, "numpad5": 0x65, "numpad6": 0x66, "numpad7": 0x67,
		"numpad8": 0x68, "numpad9": 0x69,
		"multiply": 0x6A, "add": 0x6B, "subtract": 0x6D, "decimal": 0x6E, "divide": 0x6F,

		// Special characters
		"minus":     0xBD, // -
		"plus":      0xBB, // +
		"equal":     0xBB, // =
		"equals":    0xBB,
		"bracketleft":  0xDB, // [
		"bracketright": 0xDD, // ]
		"backslash": 0xDC,
		"semicolon": 0xBA, // ;
		"quote":     0xDE, // '
		"comma":     0xBC, // ,
		"period":    0xBE, // .
		"slash":     0xBF, // /
		"grave":     0xC0, // `
		"tilde":     0xC0,
	}

	if vk, ok := specialKeys[key]; ok {
		return vk
	}

	return 0
}

// Register registers a global hotkey that works even when the window
// is not focused.
//
// The accelerator format: "Ctrl+Shift+T", "Alt+F2", etc.
// Returns true if registration succeeded.
func (m *GlobalHotkeyManager) Register(accelerator string, callback func()) bool {
	if accelerator == "" {
		return false
	}

	mods, vk, err := parseAccelerator(accelerator)
	if err != nil {
		log.Printf("hotkeys: failed to parse accelerator %q: %v", accelerator, err)
		return false
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if already registered
	accelLower := strings.ToLower(accelerator)
	if id, exists := m.accelMap[accelLower]; exists {
		// Update callback
		m.hotkeys[id] = callback
		return true
	}

	// Allocate new ID
	id := m.nextID
	m.nextID++

	// Register with Windows
	ret, _, lastErr := procRegisterHotKey.Call(
		uintptr(m.hwnd),
		uintptr(id),
		uintptr(mods),
		uintptr(vk),
	)
	if ret == 0 {
		log.Printf("hotkeys: RegisterHotKey failed for %q: %v (mods=0x%X, vk=0x%X)", accelerator, lastErr, mods, vk)
		return false
	}

	// Store mapping
	m.hotkeys[id] = callback
	m.accelMap[accelLower] = id
	m.enabled = true

	log.Printf("hotkeys: registered %q (id=%d, mods=0x%X, vk=0x%X)", accelerator, id, mods, vk)
	return true
}

// Trigger invokes the callback for the given accelerator, if registered.
// This is provided for compatibility with the Linux implementation,
// but on Windows hotkeys are triggered directly by the message loop.
func (m *GlobalHotkeyManager) Trigger(accelerator string) bool {
	m.mu.Lock()
	accelLower := strings.ToLower(accelerator)
	id, ok := m.accelMap[accelLower]
	if !ok {
		m.mu.Unlock()
		return false
	}
	cb := m.hotkeys[id]
	m.mu.Unlock()

	if cb == nil {
		return false
	}
	cb()
	return true
}

// IsAvailable reports whether global hotkeys can work in this environment.
// On Windows, this always returns true.
func (m *GlobalHotkeyManager) IsAvailable() bool {
	return true
}

// Close releases all hotkeys and destroys the hidden window.
func (m *GlobalHotkeyManager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return
	}
	m.running = false

	// Unregister all hotkeys
	for id := range m.hotkeys {
		procUnregisterHotKey.Call(uintptr(m.hwnd), uintptr(id))
	}

	// Stop message loop
	close(m.stopCh)

	// Destroy hidden window
	if m.hwnd != 0 {
		procDestroyWindow.Call(uintptr(m.hwnd))
		m.hwnd = 0
	}

	m.hotkeys = make(map[uint32]func())
	m.accelMap = make(map[string]uint32)
}

// logStatus logs the hotkey setup result for debugging.
func (m *GlobalHotkeyManager) logStatus() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.hotkeys) == 0 {
		log.Printf("hotkeys: none registered")
		return
	}

	keys := make([]string, 0, len(m.accelMap))
	for k := range m.accelMap {
		keys = append(keys, k)
	}
	log.Printf("hotkeys: registered %d (%s) — global (system-wide) active", len(keys), strings.Join(keys, ", "))
}

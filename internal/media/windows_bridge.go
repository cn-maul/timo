//go:build windows

package media

import (
	"fmt"
	"syscall"
	"unsafe"
)

// windowsBridge wraps media_bridge.dll via syscall.LazyDLL.
// The DLL is loaded at runtime; if absent, BridgeAvailable() returns false
// and media features are unavailable (app.go handles graceful degradation).
//
// Build the DLL: see build/windows/build_bridge.bat
// DLL must be in the same directory as the timo.exe binary.

var (
	bridgeDLL = syscall.NewLazyDLL("media_bridge.dll")

	bridgeFreeString     = bridgeDLL.NewProc("BridgeFreeString")
	bridgeGetSessions    = bridgeDLL.NewProc("BridgeGetSessionsList")
	bridgeGetMediaState  = bridgeDLL.NewProc("BridgeGetMediaState")
	bridgeGetCapabilities = bridgeDLL.NewProc("BridgeGetCapabilities")
	bridgePlay           = bridgeDLL.NewProc("BridgeControlPlay")
	bridgePause          = bridgeDLL.NewProc("BridgeControlPause")
	bridgeNext           = bridgeDLL.NewProc("BridgeControlNext")
	bridgePrevious       = bridgeDLL.NewProc("BridgeControlPrevious")
	bridgeSeek           = bridgeDLL.NewProc("BridgeControlSeek")
	bridgeSetShuffle     = bridgeDLL.NewProc("BridgeControlSetShuffle")
	bridgeSetRepeat      = bridgeDLL.NewProc("BridgeControlSetRepeat")
)

// BridgeAvailable checks whether media_bridge.dll loaded successfully.
func BridgeAvailable() bool {
	return bridgeDLL.Load() == nil
}

// ── string helpers ──

// ptrToString converts a null-terminated ANSI char* returned from the bridge to a Go string.
func ptrToString(p uintptr) string {
	if p == 0 {
		return ""
	}
	buf := make([]byte, 0)
	for {
		b := *(*byte)(unsafe.Pointer(p))
		if b == 0 {
			break
		}
		buf = append(buf, b)
		p++
	}
	return string(buf)
}

// freeString frees a char* allocated by the bridge.
func freeString(p uintptr) {
	if p != 0 {
		bridgeFreeString.Call(p)
	}
}

// callBridgeJSON calls a bridge function that returns a JSON string via char** outJson.
// An optional sessionID parameter may be passed for session-targeted queries.
// Returns the parsed JSON string, or an error if the HRESULT failed.
func callBridgeJSON(proc *syscall.LazyProc, sessionID ...string) (string, error) {
	var out uintptr
	var args []uintptr
	if len(sessionID) > 0 && sessionID[0] != "" {
		args = append(args, uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(sessionID[0]))))
	}
	args = append(args, uintptr(unsafe.Pointer(&out)))

	ret, _, _ := proc.Call(args...)
	if failed(ret) && out == 0 {
		return "", fmt.Errorf("bridge call failed: hr=0x%08x", ret)
	}
	defer freeString(out)

	result := ptrToString(out)
	if failed(ret) {
		return result, fmt.Errorf("bridge call failed: hr=0x%08x: %s", ret, result)
	}
	return result, nil
}

// callBridgeControl calls a bridge control function that takes sessionID.
func callBridgeControl(proc *syscall.LazyProc, sessionID string) error {
	var wID uintptr
	if sessionID != "" {
		wID = uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(sessionID)))
	}

	ret, _, _ := proc.Call(wID)
	if succeeded(ret) {
		return nil
	}
	return fmt.Errorf("bridge control call failed: hr=0x%08x", ret)
}

// ── Public bridge API ──

func BridgeGetSessionsList() (string, error) {
	return callBridgeJSON(bridgeGetSessions)
}

func BridgeGetMediaState(sessionID string) (string, error) {
	return callBridgeJSON(bridgeGetMediaState, sessionID)
}

func BridgeGetCapabilities(sessionID string) (string, error) {
	return callBridgeJSON(bridgeGetCapabilities, sessionID)
}

func BridgePlay(sessionID string) error {
	return callBridgeControl(bridgePlay, sessionID)
}

func BridgePause(sessionID string) error {
	return callBridgeControl(bridgePause, sessionID)
}

func BridgeNext(sessionID string) error {
	return callBridgeControl(bridgeNext, sessionID)
}

func BridgePrevious(sessionID string) error {
	return callBridgeControl(bridgePrevious, sessionID)
}

func BridgeSeek(sessionID string, positionMs int64) error {
	var wID uintptr
	if sessionID != "" {
		wID = uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(sessionID)))
	}
	ret, _, _ := bridgeSeek.Call(wID, uintptr(positionMs))
	if succeeded(ret) {
		return nil
	}
	return fmt.Errorf("bridge seek failed: hr=0x%08x", ret)
}

func BridgeSetShuffle(sessionID string, enabled bool) error {
	var wID uintptr
	if sessionID != "" {
		wID = uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(sessionID)))
	}
	e := uintptr(0)
	if enabled {
		e = 1
	}
	ret, _, _ := bridgeSetShuffle.Call(wID, e)
	if succeeded(ret) {
		return nil
	}
	return fmt.Errorf("bridge set shuffle failed: hr=0x%08x", ret)
}

func BridgeSetRepeat(sessionID string, mode int) error {
	var wID uintptr
	if sessionID != "" {
		wID = uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(sessionID)))
	}
	ret, _, _ := bridgeSetRepeat.Call(wID, uintptr(mode))
	if succeeded(ret) {
		return nil
	}
	return fmt.Errorf("bridge set repeat failed: hr=0x%08x", ret)
}

// ── HRESULT helpers (unexported) ──

func succeeded(hr uintptr) bool {
	return int32(hr) >= 0
}

func failed(hr uintptr) bool {
	return int32(hr) < 0
}

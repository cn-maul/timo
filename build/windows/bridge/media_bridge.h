// media_bridge.h — C API for Windows GSMTC native bridge.
// C++/WinRT DLL that replaces PowerShell-based media control.
//
// Build: see build/windows/CMakeLists.txt or build/windows/build_bridge.bat
//
// All functions are thread-safe (WinRT apartment handles synchronization).
// String outputs (char**) are allocated with CoTaskMemAlloc; caller must
// free them with BridgeFreeString().

#pragma once

#include <windows.h>
#include <stdint.h>

#ifdef MEDIA_BRIDGE_EXPORTS
#define BRIDGE_API __declspec(dllexport)
#else
#define BRIDGE_API __declspec(dllimport)
#endif

extern "C" {

// ── Memory ──

// Frees a string previously returned by any BridgeGet* function.
BRIDGE_API void WINAPI BridgeFreeString(char* str);

// ── Session enumeration ──

// Returns JSON array of media sessions: [{"id":"...","name":"...","playing":true/false}]
// Returns S_OK on success, E_FAIL on error.
BRIDGE_API HRESULT WINAPI BridgeGetSessionsList(char** outJson);

// ── Media state ──

// Returns JSON with current media state for the given session:
// {"title":"...","artist":"...","album":"...","coverBase64":"...",
//  "durationMs":123,"positionMs":456,"playing":true,"sessionId":"..."}
// Pass NULL or L"" for the current / highest-priority session.
BRIDGE_API HRESULT WINAPI BridgeGetMediaState(const wchar_t* sessionId, char** outJson);

// ── Capabilities ──

// Returns JSON: {"canPlay":true,"canPause":true,"canNext":true,
//                "canPrevious":true,"canSeek":false,"canShuffle":false,"canRepeat":false}
BRIDGE_API HRESULT WINAPI BridgeGetCapabilities(const wchar_t* sessionId, char** outJson);

// ── Playback control ──

// All control functions return S_OK on success, S_FALSE if the action is not
// supported by the current media, or E_FAIL on error.
BRIDGE_API HRESULT WINAPI BridgeControlPlay(const wchar_t* sessionId);
BRIDGE_API HRESULT WINAPI BridgeControlPause(const wchar_t* sessionId);
BRIDGE_API HRESULT WINAPI BridgeControlNext(const wchar_t* sessionId);
BRIDGE_API HRESULT WINAPI BridgeControlPrevious(const wchar_t* sessionId);

// SeeMs to the specified position (in milliseconds).
BRIDGE_API HRESULT WINAPI BridgeControlSeek(const wchar_t* sessionId, int64_t positionMs);

// Set shuffle mode. enabled: TRUE = shuffle on, FALSE = off.
BRIDGE_API HRESULT WINAPI BridgeControlSetShuffle(const wchar_t* sessionId, BOOL enabled);

// Set repeat mode. mode: 0 = None, 1 = One (Track), 2 = All (List).
BRIDGE_API HRESULT WINAPI BridgeControlSetRepeat(const wchar_t* sessionId, int mode);

} // extern "C"

// media_bridge.cpp — C++/WinRT implementation of the GSMTC native bridge.
//
// Requires: Windows 10 SDK (10.0.19041+) with C++/WinRT headers
// Build:     cl /EHsc /std:c++17 /D MEDIA_BRIDGE_EXPORTS /LD
//               media_bridge.cpp /link /OUT:media_bridge.dll

#include <winrt/Windows.Media.Control.h>
#include <winrt/Windows.Foundation.Collections.h>
#include <winrt/Windows.Storage.Streams.h>

#include <windows.h>
#include <objbase.h>

#include <string>
#include <vector>
#include <sstream>

using namespace winrt;
using namespace Windows::Media::Control;
using namespace Windows::Storage::Streams;

// ── Helpers ──

static bool EnsureApartment() {
    static bool init = [] {
        winrt::init_apartment(winrt::apartment_type::multi_threaded);
        return true;
    }();
    return init;
}

static GlobalSystemMediaTransportControlsSessionManager GetManager() {
    return GlobalSystemMediaTransportControlsSessionManager::RequestAsync()
        .get();
}

// Find a session by its SourceAppUserModelId, or return the first session.
static GlobalSystemMediaTransportControlsSession FindSession(
    const std::wstring& targetId)
{
    auto manager = GetManager();
    auto sessions = manager.GetSessions();

    if (!targetId.empty()) {
        for (auto&& s : sessions) {
            if (s.SourceAppUserModelId() == targetId) {
                return s;
            }
        }
    }

    if (sessions.Size() > 0) {
        return sessions.GetAt(0);
    }
    return nullptr;
}

// Allocate a char* string that the caller frees via BridgeFreeString.
// Uses CoTaskMemAlloc so it works seamlessly with Go's syscall layer.
// Throws std::bad_alloc on failure.
static char* AllocString(const std::string& s) {
    size_t len = s.size() + 1;
    char* buf = static_cast<char*>(CoTaskMemAlloc(len));
    if (!buf) throw std::bad_alloc();
    memcpy(buf, s.c_str(), len);
    return buf;
}

// ── JSON builders ──

// Forward declaration
static std::string escape_json(const std::string& s);

static std::string BuildSessionsJson(
    const winrt::Windows::Foundation::Collections::IVectorView<
        GlobalSystemMediaTransportControlsSession>& sessions)
{
    std::ostringstream json;
    json << "[";
    bool first = true;
    for (auto&& s : sessions) {
        if (!first) json << ",";
        first = false;

        auto playback = s.GetPlaybackInfo();
        bool playing = playback.PlaybackStatus() ==
            GlobalSystemMediaTransportControlsSessionPlaybackStatus::Playing;

        std::string id = winrt::to_string(s.SourceAppUserModelId());
        std::string name = id;

        // Extract a friendly name from the AppUserModelId
        auto dot = id.find_last_of('.');
        auto bang = id.find('!');
        if (dot != std::string::npos && dot > 0 && dot < id.size() - 1) {
            name = id.substr(dot + 1);
        }
        if (bang != std::string::npos) {
            name = name.substr(0, bang);
        }
        // Truncate hash suffixes (publisher hash after underscore)
        auto us = name.find('_');
        if (us != std::string::npos && name.size() - us > 20) {
            name = name.substr(0, us);
        }

        json << "{\"id\":\"" << escape_json(id)
             << "\",\"name\":\"" << escape_json(name)
             << "\",\"playing\":" << (playing ? "true" : "false")
             << "}";
    }
    json << "]";
    return json.str();
}

static std::string escape_json(const std::string& s) {
    std::string out;
    out.reserve(s.size());
    for (char c : s) {
        switch (c) {
            case '"': out += "\\\""; break;
            case '\\': out += "\\\\"; break;
            case '\n': out += "\\n"; break;
            case '\r': out += "\\r"; break;
            case '\t': out += "\\t"; break;
            default: out += c;
        }
    }
    return out;
}

// ── Exported API ──

extern "C" {

BRIDGE_API void WINAPI BridgeFreeString(char* str) {
    if (str) CoTaskMemFree(str);
}

BRIDGE_API HRESULT WINAPI BridgeGetSessionsList(char** outJson) {
    if (!outJson) return E_POINTER;
    *outJson = nullptr;

    try {
        EnsureApartment();
        auto manager = GetManager();
        auto sessions = manager.GetSessions();
        auto json = BuildSessionsJson(sessions);
        *outJson = AllocString(json);
        return S_OK;
    } catch (const winrt::hresult_error& e) {
        *outJson = AllocString("{\"error\":\"" +
            escape_json(winrt::to_string(e.message())) + "\"}");
        return e.code();
    } catch (...) {
        return E_FAIL;
    }
}

BRIDGE_API HRESULT WINAPI BridgeGetMediaState(const wchar_t* sessionId, char** outJson) {
    if (!outJson) return E_POINTER;
    *outJson = nullptr;

    try {
        EnsureApartment();
        std::wstring target = sessionId ? sessionId : L"";
        auto session = FindSession(target);
        if (!session) {
            *outJson = AllocString("{\"error\":\"No active media session\"}");
            return S_FALSE;
        }

        auto props = session.TryGetMediaPropertiesAsync().get();
        auto playback = session.GetPlaybackInfo();
        auto timeline = session.GetTimelineProperties();

        std::string title = winrt::to_string(props.Title());
        std::string artist = winrt::to_string(props.Artist());
        std::string album = winrt::to_string(props.AlbumTitle());
        std::string albumArtist = winrt::to_string(props.AlbumArtist());

        // Fallback: use album artist when artist is empty
        if (artist.empty() && !albumArtist.empty()) {
            artist = albumArtist;
        }

        // Cover art: thumbnail as base64
        std::string coverBase64;
        auto thumbnail = props.Thumbnail();
        if (thumbnail) {
            try {
                auto stream = thumbnail.OpenReadAsync().get();
                if (stream && stream.Size() > 0) {
                    uint32_t size = static_cast<uint32_t>(stream.Size());
                    auto reader = DataReader(stream.GetInputStreamAt(0));
                    reader.LoadAsync(size).get();
                    std::vector<uint8_t> data(size);
                    reader.ReadBytes(data);

                    // Base64 encode
                    static const char b64[] =
                        "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
                    std::string b64out;
                    b64out.reserve(((size + 2) / 3) * 4);
                    for (size_t i = 0; i < size; i += 3) {
                        uint32_t v = (uint32_t)data[i] << 16;
                        if (i + 1 < size) v |= (uint32_t)data[i + 1] << 8;
                        if (i + 2 < size) v |= (uint32_t)data[i + 2];
                        b64out += b64[(v >> 18) & 0x3F];
                        b64out += b64[(v >> 12) & 0x3F];
                        b64out += (i + 1 < size) ? b64[(v >> 6) & 0x3F] : '=';
                        b64out += (i + 2 < size) ? b64[v & 0x3F] : '=';
                    }
                    coverBase64 = std::move(b64out);
                }
            } catch (...) {
                // Silently ignore thumbnail errors
            }
        }

        int64_t durationMs = 0;
        if (timeline.EndTime() && timeline.EndTime().TotalMilliseconds() > 0) {
            durationMs = static_cast<int64_t>(timeline.EndTime().TotalMilliseconds());
        }
        int64_t positionMs = 0;
        if (timeline.Position() && timeline.Position().TotalMilliseconds() > 0) {
            positionMs = static_cast<int64_t>(timeline.Position().TotalMilliseconds());
        }

        bool playing = playback.PlaybackStatus() ==
            GlobalSystemMediaTransportControlsSessionPlaybackStatus::Playing;

        std::string sessionIdStr = winrt::to_string(session.SourceAppUserModelId());

        std::ostringstream json;
        json << "{"
             << "\"title\":\"" << escape_json(title) << "\","
             << "\"artist\":\"" << escape_json(artist) << "\","
             << "\"album\":\"" << escape_json(album) << "\","
             << "\"coverBase64\":\"" << coverBase64 << "\","
             << "\"durationMs\":" << durationMs << ","
             << "\"positionMs\":" << positionMs << ","
             << "\"playing\":" << (playing ? "true" : "false") << ","
             << "\"sessionId\":\"" << escape_json(sessionIdStr) << "\""
             << "}";

        *outJson = AllocString(json.str());
        return S_OK;
    } catch (const winrt::hresult_error& e) {
        *outJson = AllocString("{\"error\":\"" +
            escape_json(winrt::to_string(e.message())) + "\"}");
        return e.code();
    } catch (...) {
        return E_FAIL;
    }
}

BRIDGE_API HRESULT WINAPI BridgeGetCapabilities(const wchar_t* sessionId, char** outJson) {
    if (!outJson) return E_POINTER;
    *outJson = nullptr;

    try {
        EnsureApartment();
        std::wstring target = sessionId ? sessionId : L"";
        auto session = FindSession(target);
        if (!session) {
            *outJson = AllocString("{\"error\":\"No active session\"}");
            return S_FALSE;
        }

        auto playback = session.GetPlaybackInfo();
        auto controls = playback.Controls();

        std::ostringstream json;
        json << "{"
             << "\"canPlay\":" << (controls.IsPlayEnabled() ? "true" : "false") << ","
             << "\"canPause\":" << (controls.IsPauseEnabled() ? "true" : "false") << ","
             << "\"canNext\":" << (controls.IsNextEnabled() ? "true" : "false") << ","
             << "\"canPrevious\":" << (controls.IsPreviousEnabled() ? "true" : "false") << ","
             << "\"canSeek\":" << (controls.IsPlaybackPositionEnabled() ? "true" : "false") << ","
             << "\"canShuffle\":" << (controls.IsShuffleEnabled() ? "true" : "false") << ","
             << "\"canRepeat\":" << (controls.IsRepeatEnabled() ? "true" : "false") << ""
             << "}";

        *outJson = AllocString(json.str());
        return S_OK;
    } catch (const winrt::hresult_error& e) {
        *outJson = AllocString("{\"error\":\"" +
            escape_json(winrt::to_string(e.message())) + "\"}");
        return e.code();
    } catch (...) {
        return E_FAIL;
    }
}

// ── Control helpers ──

static HRESULT ControlAction(const wchar_t* sessionId,
    std::function<winrt::Windows::Foundation::IAsyncOperation<bool>(GlobalSystemMediaTransportControlsSession&)> action)
{
    try {
        EnsureApartment();
        std::wstring target = sessionId ? sessionId : L"";
        auto session = FindSession(target);
        if (!session) return S_FALSE;

        bool result = action(session).get();
        return result ? S_OK : S_FALSE;
    } catch (const winrt::hresult_error&) {
        return S_FALSE;
    } catch (...) {
        return E_FAIL;
    }
}

BRIDGE_API HRESULT WINAPI BridgeControlPlay(const wchar_t* sessionId) {
    return ControlAction(sessionId, [](auto& s) { return s.TryPlayAsync(); });
}

BRIDGE_API HRESULT WINAPI BridgeControlPause(const wchar_t* sessionId) {
    return ControlAction(sessionId, [](auto& s) { return s.TryPauseAsync(); });
}

BRIDGE_API HRESULT WINAPI BridgeControlNext(const wchar_t* sessionId) {
    return ControlAction(sessionId, [](auto& s) { return s.TrySkipNextAsync(); });
}

BRIDGE_API HRESULT WINAPI BridgeControlPrevious(const wchar_t* sessionId) {
    return ControlAction(sessionId, [](auto& s) { return s.TrySkipPreviousAsync(); });
}

BRIDGE_API HRESULT WINAPI BridgeControlSeek(const wchar_t* sessionId, int64_t positionMs) {
    try {
        EnsureApartment();
        std::wstring target = sessionId ? sessionId : L"";
        auto session = FindSession(target);
        if (!session) return S_FALSE;

        // GSMTC uses 100-nanosecond ticks
        auto ticks = positionMs * 10000;
        bool result = session.TryChangePlaybackPositionAsync(ticks).get();
        return result ? S_OK : S_FALSE;
    } catch (...) {
        return E_FAIL;
    }
}

BRIDGE_API HRESULT WINAPI BridgeControlSetShuffle(const wchar_t* sessionId, BOOL enabled) {
    return ControlAction(sessionId, [enabled](auto& s) {
        return s.TryChangeShuffleEnabledAsync(enabled ? true : false);
    });
}

BRIDGE_API HRESULT WINAPI BridgeControlSetRepeat(const wchar_t* sessionId, int mode) {
    try {
        EnsureApartment();
        std::wstring target = sessionId ? sessionId : L"";
        auto session = FindSession(target);
        if (!session) return S_FALSE;

        // GSMTC only supports toggle on/off for repeat, not track vs list
        bool enabled = mode != 0;
        bool result = session.TryChangeRepeatEnabledAsync(enabled).get();
        return result ? S_OK : S_FALSE;
    } catch (...) {
        return E_FAIL;
    }
}

} // extern "C"

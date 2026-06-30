//go:build windows

package media

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Player priority configuration — lower number = higher priority
var playerPriority = map[string]int{
	"SpotifyAB.SpotifyMusic_": 1,
	"Spotify":                  1,
	"AppleInc.AppleMusic":         2,
	"Microsoft.ZuneMusic":          3,
	"Microsoft.WindowsMediaPlayer": 3,
	"Music.UI":                    3,
	"Video.UI": 4,
	"vlc":       5,
	"VLC":       5,
	"chrome.exe":    10,
	"msedge.exe":    11,
	"firefox.exe":   12,
	"Chrome":        10,
	"MicrosoftEdge": 11,
	"Firefox":       12,
	"default": 100,
}

// WindowsProvider implements MediaProvider using native GSMTC (media_bridge.dll).
type WindowsProvider struct {
	mu sync.Mutex

	cache          *MediaInfo
	cacheTime      time.Time
	cacheTTL       time.Duration
	cacheSessionID string

	lastSessionID string
}

const defaultCacheTTL = 500 * time.Millisecond

// gsmrtcMediaInfo represents the JSON output from the bridge for a single session
type gsmrtcMediaInfo struct {
	Title       string `json:"title"`
	Artist      string `json:"artist"`
	Album       string `json:"album"`
	CoverBase64 string `json:"coverBase64"`
	DurationMs  int64  `json:"durationMs"`
	PositionMs  int64  `json:"positionMs"`
	Playing     bool   `json:"playing"`
	SessionID   string `json:"sessionId"`
}

// gsmrtcSessionInfo represents a session from the sessions list
type gsmrtcSessionInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Playing bool   `json:"playing"`
}

// NewWindowsProvider creates a new Windows media provider backed by media_bridge.dll.
// Returns an error if the DLL is not available (app gracefully skips media features).
func NewWindowsProvider() (MediaProvider, error) {
	if !BridgeAvailable() {
		return nil, fmt.Errorf("media_bridge.dll not found — install it from build/windows/media_bridge.dll or run build/windows/build_bridge.bat")
	}
	return &WindowsProvider{
		cacheTTL: defaultCacheTTL,
	}, nil
}

func getPlayerPriority(sessionID string) int {
	for pattern, p := range playerPriority {
		if pattern != "default" && strings.Contains(sessionID, pattern) {
			return p
		}
	}
	return playerPriority["default"]
}

func extractPlayerName(sessionID string) string {
	id := strings.Split(sessionID, "!")[0]
	if strings.Contains(id, "_") {
		parts := strings.Split(id, "_")
		if len(parts) >= 1 {
			appPart := parts[0]
			if dotIdx := strings.LastIndex(appPart, "."); dotIdx >= 0 && dotIdx < len(appPart)-1 {
				return appPart[dotIdx+1:]
			}
			return appPart
		}
	}
	if strings.HasSuffix(id, ".exe") {
		return strings.TrimSuffix(id, ".exe")
	}
	return id
}

// ── Bridge call helpers ──

func (p *WindowsProvider) bridgeSessions() ([]gsmrtcSessionInfo, error) {
	jsonStr, err := BridgeGetSessionsList()
	if err != nil {
		return nil, err
	}
	jsonStr = strings.TrimSpace(jsonStr)
	if jsonStr == "" || jsonStr == "[]" || jsonStr == "null" {
		return []gsmrtcSessionInfo{}, nil
	}

	var sessions []gsmrtcSessionInfo
	if err := json.Unmarshal([]byte(jsonStr), &sessions); err != nil {
		var single struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Playing bool   `json:"playing"`
		}
		if err2 := json.Unmarshal([]byte(jsonStr), &single); err2 == nil {
			return []gsmrtcSessionInfo{{
				ID:      single.ID,
				Name:    single.Name,
				Playing: single.Playing,
			}}, nil
		}
		return nil, fmt.Errorf("failed to parse sessions JSON: %w", err)
	}
	return sessions, nil
}

func (p *WindowsProvider) bridgeMediaState(sessionID string) (*gsmrtcMediaInfo, error) {
	jsonStr, err := BridgeGetMediaState(sessionID)
	if err != nil {
		var errResp struct {
			Error string `json:"error"`
		}
		if json.Unmarshal([]byte(jsonStr), &errResp) == nil && errResp.Error != "" {
			return nil, fmt.Errorf("%s", errResp.Error)
		}
		return nil, err
	}
	jsonStr = strings.TrimSpace(jsonStr)
	if jsonStr == "" {
		return nil, fmt.Errorf("empty response from bridge")
	}

	var info gsmrtcMediaInfo
	if err := json.Unmarshal([]byte(jsonStr), &info); err != nil {
		return nil, fmt.Errorf("failed to parse media info: %w", err)
	}
	if info.Title == "" && info.SessionID == "" {
		return nil, fmt.Errorf("no media session active")
	}
	return &info, nil
}

// ── Session selection ──

func (p *WindowsProvider) findPlayer() (string, error) {
	sessions, err := p.bridgeSessions()
	if err != nil {
		return "", err
	}
	if len(sessions) == 0 {
		return "", fmt.Errorf("no active media session")
	}

	// Prefer the last active player for stability
	for _, s := range sessions {
		if s.ID == p.lastSessionID {
			return s.ID, nil
		}
	}

	// Find best by priority (lowest number), preferring playing
	type swp struct {
		info     gsmrtcSessionInfo
		priority int
	}
	sp := make([]swp, 0, len(sessions))
	for _, si := range sessions {
		sp = append(sp, swp{info: si, priority: getPlayerPriority(si.ID)})
	}

	best := sp[0]
	for _, s := range sp[1:] {
		if s.priority < best.priority {
			best = s
		} else if s.priority == best.priority && s.info.Playing && !best.info.Playing {
			best = s
		}
	}

	p.lastSessionID = best.info.ID
	return best.info.ID, nil
}

// ── MediaProvider implementation ──

func (p *WindowsProvider) GetSessions() ([]MediaSession, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	sessions, err := p.bridgeSessions()
	if err != nil {
		return nil, err
	}

	result := make([]MediaSession, 0, len(sessions))
	for _, si := range sessions {
		result = append(result, MediaSession{
			ID:       si.ID,
			Name:     extractPlayerName(si.ID),
			Playing:  si.Playing,
			Priority: getPlayerPriority(si.ID),
		})
	}
	return result, nil
}

func (p *WindowsProvider) GetState() (*MediaInfo, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Check cache
	if p.cache != nil && time.Since(p.cacheTime) < p.cacheTTL && p.cacheSessionID == p.lastSessionID {
		cached := *p.cache
		if cached.Playing && cached.DurationMs > 0 {
			elapsed := time.Since(p.cacheTime).Milliseconds()
			if elapsed < 0 {
				elapsed = 0
			}
			cached.PositionMs = p.cache.PositionMs + elapsed
			if cached.PositionMs > cached.DurationMs {
				cached.PositionMs = cached.DurationMs
			}
		}
		return &cached, nil
	}

	sessionID, err := p.findPlayer()
	if err != nil {
		if p.cache != nil {
			cached := *p.cache
			return &cached, nil
		}
		return nil, err
	}

	info, err := p.bridgeMediaState(sessionID)
	if err != nil {
		if p.cache != nil {
			cached := *p.cache
			return &cached, nil
		}
		return nil, err
	}

	p.lastSessionID = info.SessionID
	result := &MediaInfo{
		Title:       info.Title,
		Artist:      info.Artist,
		Album:       info.Album,
		CoverBase64: info.CoverBase64,
		DurationMs:  info.DurationMs,
		PositionMs:  info.PositionMs,
		Playing:     info.Playing,
	}
	p.cache = result
	p.cacheTime = time.Now()
	p.cacheSessionID = info.SessionID
	return result, nil
}

func (p *WindowsProvider) GetCapabilities() (*MediaCapabilities, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	sessionID, err := p.findPlayer()
	if err != nil {
		return &MediaCapabilities{}, err
	}

	jsonStr, err := BridgeGetCapabilities(sessionID)
	if err != nil {
		return &MediaCapabilities{}, err
	}
	jsonStr = strings.TrimSpace(jsonStr)
	if jsonStr == "" {
		return &MediaCapabilities{}, nil
	}

	var cap MediaCapabilities
	if err := json.Unmarshal([]byte(jsonStr), &cap); err != nil {
		return &MediaCapabilities{}, fmt.Errorf("failed to parse capabilities: %w", err)
	}
	return &cap, nil
}

func (p *WindowsProvider) controlAction(action func(string) error) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	sessionID, err := p.findPlayer()
	if err != nil {
		return err
	}
	return action(sessionID)
}

func (p *WindowsProvider) Play() error     { return p.controlAction(BridgePlay) }
func (p *WindowsProvider) Pause() error    { return p.controlAction(BridgePause) }
func (p *WindowsProvider) Next() error     { return p.controlAction(BridgeNext) }
func (p *WindowsProvider) Previous() error { return p.controlAction(BridgePrevious) }

func (p *WindowsProvider) SeekTo(positionMs int64) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	sessionID, err := p.findPlayer()
	if err != nil {
		return err
	}
	return BridgeSeek(sessionID, positionMs)
}

func (p *WindowsProvider) SetShuffle(enabled bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	sessionID, err := p.findPlayer()
	if err != nil {
		return err
	}
	return BridgeSetShuffle(sessionID, enabled)
}

func (p *WindowsProvider) SetRepeat(mode int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	sessionID, err := p.findPlayer()
	if err != nil {
		return err
	}
	return BridgeSetRepeat(sessionID, mode)
}

func (p *WindowsProvider) Close() {}

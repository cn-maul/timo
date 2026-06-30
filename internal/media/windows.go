//go:build windows

package media

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Player priority configuration - lower number = higher priority
var playerPriority = map[string]int{
	// Spotify - highest priority
	"SpotifyAB.SpotifyMusic_": 1,
	"Spotify":                  1,
	// Native music players
	"AppleInc.AppleMusic":         2,
	"Microsoft.ZuneMusic":          3, // Groove Music
	"Microsoft.WindowsMediaPlayer": 3,
	"Music.UI":                    3, // Windows 11 Media Player
	// Video players
	"Video.UI": 4, // Windows 11 Media Player (video)
	"vlc":       5,
	"VLC":       5,
	// Browsers (lower priority)
	"chrome.exe":    10,
	"msedge.exe":    11,
	"firefox.exe":   12,
	"Chrome":        10,
	"MicrosoftEdge": 11,
	"Firefox":       12,
	// Default priority for unknown players
	"default": 100,
}

// WindowsProvider implements MediaProvider using GSMTC via PowerShell.
type WindowsProvider struct {
	mu sync.Mutex

	// Cache for reducing PowerShell call frequency
	cache          *MediaInfo
	cacheTime      time.Time
	cacheTTL       time.Duration
	cacheSessionID string

	// Last known session ID for stability (prefer same player)
	lastSessionID string
}

// gsmrtcMediaInfo represents the JSON output from the PowerShell script
type gsmrtcMediaInfo struct {
	Title       string `json:"Title"`
	Artist      string `json:"Artist"`
	Album       string `json:"Album"`
	CoverBase64 string `json:"CoverBase64"`
	DurationMs  int64  `json:"DurationMs"`
	PositionMs  int64  `json:"PositionMs"`
	Playing     bool   `json:"Playing"`
	SessionID   string `json:"SessionId"`
}

// gsmrtcSessionInfo represents a session from GetSessions
type gsmrtcSessionInfo struct {
	ID      string `json:"ID"`
	Name    string `json:"Name"`
	Playing bool   `json:"Playing"`
}

// Default cache TTL for GetState calls (500ms provides good balance between
// responsiveness and reducing PowerShell spawn overhead)
const defaultCacheTTL = 500 * time.Millisecond

// NewWindowsProvider creates a new Windows media provider.
func NewWindowsProvider() (MediaProvider, error) {
	// Verify PowerShell is available
	_, err := exec.LookPath("powershell.exe")
	if err != nil {
		return nil, fmt.Errorf("powershell.exe not found: %w", err)
	}

	return &WindowsProvider{
		cacheTTL: defaultCacheTTL,
	}, nil
}

// runPowerShell executes a PowerShell script and returns its output.
func (p *WindowsProvider) runPowerShell(script string) ([]byte, error) {
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("powershell error: %s: %s", err, string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("powershell error: %w", err)
	}

	return output, nil
}

// getPlayerPriority returns the priority for a given session ID
func getPlayerPriority(sessionID string) int {
	for pattern, priority := range playerPriority {
		if pattern != "default" && strings.Contains(sessionID, pattern) {
			return priority
		}
	}
	return playerPriority["default"]
}

// extractPlayerName extracts a friendly name from the session ID
func extractPlayerName(sessionID string) string {
	// Handle App User Model IDs like "SpotifyAB.SpotifyMusic_zpdnekdrzrea0!App"
	// or "chrome.exe" etc.

	// Remove !App suffix if present
	id := strings.Split(sessionID, "!")[0]

	// Try to extract the app name from package format
	if strings.Contains(id, "_") {
		// Package format: Publisher.AppName_hash
		parts := strings.Split(id, "_")
		if len(parts) >= 1 {
			appPart := parts[0]
			// Remove publisher prefix if present
			if dotIdx := strings.LastIndex(appPart, "."); dotIdx >= 0 && dotIdx < len(appPart)-1 {
				return appPart[dotIdx+1:]
			}
			return appPart
		}
	}

	// Handle .exe names
	if strings.HasSuffix(id, ".exe") {
		return strings.TrimSuffix(id, ".exe")
	}

	return id
}

// getSessionsListScript returns the PowerShell script to get all media sessions
func (p *WindowsProvider) getSessionsListScript() string {
	return `
Add-Type -AssemblyName Windows.Media.Control -ErrorAction SilentlyContinue
if (-not ('Windows.Media.Control.GlobalSystemMediaTransportControlsSessionManager' -as [type])) {
	$result = @()
	$result | ConvertTo-Json -Compress
	exit
}

$manager = [Windows.Media.Control.GlobalSystemMediaTransportControlsSessionManager]::RequestAsync().GetAwaiter().GetResult()
$sessions = $manager.GetSessions()

$result = @()
foreach ($session in $sessions) {
	$playbackInfo = $session.GetPlaybackInfo()
	$playing = $false
	if ($playbackInfo.PlaybackStatus) {
		$playing = ($playbackInfo.PlaybackStatus -eq 4)
	}

	# Extract app name from AppUserModelId
	$appId = $session.SourceAppUserModelId
	$appName = $appId
	if ($appId -match "^([^!]+)") {
		$appName = $matches[1]
	}
	if ($appName -match "^([^._]+)") {
		$appName = $matches[1]
	}

	$result += @{
		ID      = $appId
		Name    = $appName
		Playing = $playing
	}
}

$result | ConvertTo-Json -Compress
`
}

// getSessionsInternal returns session info without locking (internal use)
func (p *WindowsProvider) getSessionsInternal() ([]gsmrtcSessionInfo, error) {
	output, err := p.runPowerShell(p.getSessionsListScript())
	if err != nil {
		return nil, fmt.Errorf("failed to get sessions: %w", err)
	}

	outputStr := strings.TrimSpace(string(output))
	if outputStr == "" || outputStr == "[]" {
		return []gsmrtcSessionInfo{}, nil
	}

	var sessionInfos []gsmrtcSessionInfo
	if err := json.Unmarshal([]byte(outputStr), &sessionInfos); err != nil {
		// Try single object case
		var singleSession gsmrtcSessionInfo
		if err2 := json.Unmarshal([]byte(outputStr), &singleSession); err2 == nil {
			sessionInfos = []gsmrtcSessionInfo{singleSession}
		} else {
			return nil, fmt.Errorf("failed to parse sessions JSON: %w", err)
		}
	}

	return sessionInfos, nil
}

// findPlayer returns the best session ID based on priority and stability.
// It prefers the last active player for stability when multiple players are running.
func (p *WindowsProvider) findPlayer() (string, error) {
	sessionInfos, err := p.getSessionsInternal()
	if err != nil {
		return "", err
	}

	if len(sessionInfos) == 0 {
		return "", fmt.Errorf("no active media session")
	}

	// Check if last session still exists and prefer it for stability
	for _, s := range sessionInfos {
		if s.ID == p.lastSessionID {
			return s.ID, nil
		}
	}

	// Sort by priority and pick the best one
	type sessionWithPriority struct {
		info     gsmrtcSessionInfo
		priority int
	}

	sessionsWithPriority := make([]sessionWithPriority, 0, len(sessionInfos))
	for _, si := range sessionInfos {
		sessionsWithPriority = append(sessionsWithPriority, sessionWithPriority{
			info:     si,
			priority: getPlayerPriority(si.ID),
		})
	}

	// Find best session (lowest priority number, prefer playing)
	best := sessionsWithPriority[0]
	for _, s := range sessionsWithPriority[1:] {
		if s.priority < best.priority {
			best = s
		} else if s.priority == best.priority && s.info.Playing && !best.info.Playing {
			best = s
		}
	}

	p.lastSessionID = best.info.ID
	return best.info.ID, nil
}

// getMediaInfoScript returns the PowerShell script to get media information for a specific session
func (p *WindowsProvider) getMediaInfoScript(sessionID string) string {
	sessionFilter := ""
	if sessionID != "" {
		sessionFilter = fmt.Sprintf(`
$targetSession = $null
foreach ($s in $sessions) {
	if ($s.SourceAppUserModelId -eq '%s') {
		$targetSession = $s
		break
	}
}
if (-not $targetSession) {
	$result = @{Error = "Session not found"}
	$result | ConvertTo-Json -Compress
	exit
}
$session = $targetSession`, sessionID)
	} else {
		sessionFilter = `
$session = $manager.GetCurrentSession()
if (-not $session) {
	$result = @{Error = "No active media session"}
	$result | ConvertTo-Json -Compress
	exit
}`
	}

	return fmt.Sprintf(`
Add-Type -AssemblyName Windows.Media.Control -ErrorAction SilentlyContinue
if (-not ('Windows.Media.Control.GlobalSystemMediaTransportControlsSessionManager' -as [type])) {
	$result = @{Error = "GSMTC not available on this system"}
	$result | ConvertTo-Json -Compress
	exit
}

$manager = [Windows.Media.Control.GlobalSystemMediaTransportControlsSessionManager]::RequestAsync().GetAwaiter().GetResult()
$sessions = $manager.GetSessions()
%s

$mediaProperties = $session.TryGetMediaPropertiesAsync().GetAwaiter().GetResult()
$playbackInfo = $session.GetPlaybackInfo()
$timelineInfo = $session.GetTimelineProperties()

$title = if ($mediaProperties.Title) { $mediaProperties.Title } else { "" }
$artist = if ($mediaProperties.Artist) { $mediaProperties.Artist } else { "" }
$album = if ($mediaProperties.AlbumTitle) { $mediaProperties.AlbumTitle } else { "" }
$albumArtist = if ($mediaProperties.AlbumArtist) { $mediaProperties.AlbumArtist } else { "" }

# Use album artist if artist is empty
if ([string]::IsNullOrEmpty($artist) -and -not [string]::IsNullOrEmpty($albumArtist)) {
	$artist = $albumArtist
}

# Extract cover image as base64
$coverBase64 = ""
if ($mediaProperties.Thumbnail) {
	try {
		$stream = $mediaProperties.Thumbnail.OpenReadAsync().GetAwaiter().GetResult()
		if ($stream -and $stream.Size -gt 0) {
			$buffer = New-Object byte[] $stream.Size
			$reader = [Windows.Storage.Streams.DataReader]::FromBuffer($stream)
			$reader.ReadBytes($buffer)
			$coverBase64 = [Convert]::ToBase64String($buffer)
		}
	} catch {
		# Silently ignore thumbnail extraction errors
	}
}

# Duration and position in milliseconds
$durationMs = 0
$positionMs = 0
if ($timelineInfo.EndTime -and $timelineInfo.EndTime.TotalMilliseconds -gt 0) {
	$durationMs = [int64]$timelineInfo.EndTime.TotalMilliseconds
}
if ($timelineInfo.Position -and $timelineInfo.Position.TotalMilliseconds -gt 0) {
	$positionMs = [int64]$timelineInfo.Position.TotalMilliseconds
}

# Playback status
$playing = $false
if ($playbackInfo.PlaybackStatus) {
	# PlaybackStatus: 0 = Closed, 1 = Opened, 2 = Changing, 3 = Stopped, 4 = Playing, 5 = Paused
	$playing = ($playbackInfo.PlaybackStatus -eq 4)
}

$result = @{
	Title       = $title
	Artist      = $artist
	Album       = $album
	CoverBase64 = $coverBase64
	DurationMs  = $durationMs
	PositionMs  = $positionMs
	Playing     = $playing
	SessionId   = $session.SourceAppUserModelId
}

$result | ConvertTo-Json -Compress
`, sessionFilter)
}

// controlPlaybackScript returns the PowerShell script for media control with session targeting
func (p *WindowsProvider) controlPlaybackScript(action string, sessionID string) string {
	sessionFilter := ""
	if sessionID != "" {
		sessionFilter = fmt.Sprintf(`
$targetSession = $null
foreach ($s in $sessions) {
	if ($s.SourceAppUserModelId -eq '%s') {
		$targetSession = $s
		break
	}
}
if (-not $targetSession) {
	Write-Error "Session not found"
	exit 1
}
$session = $targetSession`, sessionID)
	} else {
		sessionFilter = `
$session = $manager.GetCurrentSession()
if (-not $session) {
	Write-Error "No active media session"
	exit 1
}`
	}

	return fmt.Sprintf(`
Add-Type -AssemblyName Windows.Media.Control -ErrorAction SilentlyContinue
if (-not ('Windows.Media.Control.GlobalSystemMediaTransportControlsSessionManager' -as [type])) {
	Write-Error "GSMTC not available"
	exit 1
}

$manager = [Windows.Media.Control.GlobalSystemMediaTransportControlsSessionManager]::RequestAsync().GetAwaiter().GetResult()
$sessions = $manager.GetSessions()
%s

# Get playback info to check capabilities
$playbackInfo = $session.GetPlaybackInfo()
$controls = $playbackInfo.Controls

# Try to perform the requested action
try {
	switch ("%s") {
		"Play" {
			if ($controls.IsPlayEnabled) {
				$session.TryPlayAsync().GetAwaiter().GetResult() | Out-Null
			} else {
				Write-Error "Play not available for current media"
				exit 1
			}
		}
		"Pause" {
			if ($controls.IsPauseEnabled) {
				$session.TryPauseAsync().GetAwaiter().GetResult() | Out-Null
			} else {
				Write-Error "Pause not available for current media"
				exit 1
			}
		}
		"PlayPause" {
			if ($controls.IsPlayPauseToggleEnabled) {
				$session.TryTogglePlayPauseAsync().GetAwaiter().GetResult() | Out-Null
			} elseif ($controls.IsPlayEnabled) {
				$session.TryPlayAsync().GetAwaiter().GetResult() | Out-Null
			} elseif ($controls.IsPauseEnabled) {
				$session.TryPauseAsync().GetAwaiter().GetResult() | Out-Null
			} else {
				Write-Error "Play/Pause not available for current media"
				exit 1
			}
		}
		"Next" {
			if ($controls.IsNextEnabled) {
				$session.TrySkipNextAsync().GetAwaiter().GetResult() | Out-Null
			} else {
				Write-Error "Next not available for current media"
				exit 1
			}
		}
		"Previous" {
			if ($controls.IsPreviousEnabled) {
				$session.TrySkipPreviousAsync().GetAwaiter().GetResult() | Out-Null
			} else {
				Write-Error "Previous not available for current media"
				exit 1
			}
		}
	}
} catch {
	Write-Error ("Control action failed: " + $_.Exception.Message)
	exit 1
}
`, sessionFilter, action)
}

// getCapabilitiesScript returns PowerShell script to check media capabilities for a specific session
func (p *WindowsProvider) getCapabilitiesScript(sessionID string) string {
	sessionFilter := ""
	if sessionID != "" {
		sessionFilter = fmt.Sprintf(`
$targetSession = $null
foreach ($s in $sessions) {
	if ($s.SourceAppUserModelId -eq '%s') {
		$targetSession = $s
		break
	}
}
if (-not $targetSession) {
	$result = @{CanPlay=$false; CanPause=$false; CanNext=$false; CanPrevious=$false; CanSeek=$false; CanShuffle=$false; CanRepeat=$false}
	$result | ConvertTo-Json -Compress
	exit
}
$session = $targetSession`, sessionID)
	} else {
		sessionFilter = `
$session = $manager.GetCurrentSession()
if (-not $session) {
	$result = @{CanPlay=$false; CanPause=$false; CanNext=$false; CanPrevious=$false; CanSeek=$false; CanShuffle=$false; CanRepeat=$false}
	$result | ConvertTo-Json -Compress
	exit
}`
	}

	return fmt.Sprintf(`
Add-Type -AssemblyName Windows.Media.Control -ErrorAction SilentlyContinue
if (-not ('Windows.Media.Control.GlobalSystemMediaTransportControlsSessionManager' -as [type])) {
	$result = @{CanPlay=$false; CanPause=$false; CanNext=$false; CanPrevious=$false; CanSeek=$false; CanShuffle=$false; CanRepeat=$false}
	$result | ConvertTo-Json -Compress
	exit
}

$manager = [Windows.Media.Control.GlobalSystemMediaTransportControlsSessionManager]::RequestAsync().GetAwaiter().GetResult()
$sessions = $manager.GetSessions()
%s

$playbackInfo = $session.GetPlaybackInfo()
$controls = $playbackInfo.Controls

$result = @{
	CanPlay     = $controls.IsPlayEnabled
	CanPause    = $controls.IsPauseEnabled
	CanNext     = $controls.IsNextEnabled
	CanPrevious = $controls.IsPreviousEnabled
	CanSeek     = $controls.IsPlaybackPositionEnabled
	CanShuffle  = $controls.IsShuffleEnabled
	CanRepeat   = $controls.IsRepeatEnabled
}

$result | ConvertTo-Json -Compress
`, sessionFilter)
}

// seekScript returns the PowerShell script for seeking to a specific session
func (p *WindowsProvider) seekScript(positionMs int64, sessionID string) string {
	// Position in GSMTC is in 100-nanosecond units (ticks)
	// 1 ms = 10,000 ticks
	positionTicks := positionMs * 10000

	sessionFilter := ""
	if sessionID != "" {
		sessionFilter = fmt.Sprintf(`
$targetSession = $null
foreach ($s in $sessions) {
	if ($s.SourceAppUserModelId -eq '%s') {
		$targetSession = $s
		break
	}
}
if (-not $targetSession) {
	Write-Error "Session not found"
	exit 1
}
$session = $targetSession`, sessionID)
	} else {
		sessionFilter = `
$session = $manager.GetCurrentSession()
if (-not $session) {
	Write-Error "No active media session"
	exit 1
}`
	}

	return fmt.Sprintf(`
Add-Type -AssemblyName Windows.Media.Control -ErrorAction SilentlyContinue
if (-not ('Windows.Media.Control.GlobalSystemMediaTransportControlsSessionManager' -as [type])) {
	Write-Error "GSMTC not available"
	exit 1
}

$manager = [Windows.Media.Control.GlobalSystemMediaTransportControlsSessionManager]::RequestAsync().GetAwaiter().GetResult()
$sessions = $manager.GetSessions()
%s

# Try to change playback position (position is in 100-nanosecond units)
try {
	$position = [TimeSpan]::FromTicks([int64]%d)
	$session.TryChangePlaybackPositionAsync($position.Ticks).GetAwaiter().GetResult() | Out-Null
} catch {
	Write-Error ("Seek failed: " + $_.Exception.Message)
	exit 1
}
`, sessionFilter, positionTicks)
}

// setShuffleScript returns the PowerShell script for setting shuffle mode
func (p *WindowsProvider) setShuffleScript(enabled bool, sessionID string) string {
	enabledStr := "$false"
	if enabled {
		enabledStr = "$true"
	}

	sessionFilter := ""
	if sessionID != "" {
		sessionFilter = fmt.Sprintf(`
$targetSession = $null
foreach ($s in $sessions) {
	if ($s.SourceAppUserModelId -eq '%s') {
		$targetSession = $s
		break
	}
}
if (-not $targetSession) {
	Write-Error "Session not found"
	exit 1
}
$session = $targetSession`, sessionID)
	} else {
		sessionFilter = `
$session = $manager.GetCurrentSession()
if (-not $session) {
	Write-Error "No active media session"
	exit 1
}`
	}

	return fmt.Sprintf(`
Add-Type -AssemblyName Windows.Media.Control -ErrorAction SilentlyContinue
if (-not ('Windows.Media.Control.GlobalSystemMediaTransportControlsSessionManager' -as [type])) {
	Write-Error "GSMTC not available"
	exit 1
}

$manager = [Windows.Media.Control.GlobalSystemMediaTransportControlsSessionManager]::RequestAsync().GetAwaiter().GetResult()
$sessions = $manager.GetSessions()
%s

# Try to set shuffle mode
try {
	$session.TryChangeShuffleEnabledAsync(%s).GetAwaiter().GetResult() | Out-Null
} catch {
	Write-Error ("Set shuffle failed: " + $_.Exception.Message)
	exit 1
}
`, sessionFilter, enabledStr)
}

// setRepeatScript returns the PowerShell script for setting repeat mode
func (p *WindowsProvider) setRepeatScript(mode int, sessionID string) string {
	// MediaPlayerRepeatMode: 0 = None, 1 = Track (One), 2 = List (All)
	enabled := "$false"
	if mode != 0 {
		enabled = "$true"
	}

	sessionFilter := ""
	if sessionID != "" {
		sessionFilter = fmt.Sprintf(`
$targetSession = $null
foreach ($s in $sessions) {
	if ($s.SourceAppUserModelId -eq '%s') {
		$targetSession = $s
		break
	}
}
if (-not $targetSession) {
	Write-Error "Session not found"
	exit 1
}
$session = $targetSession`, sessionID)
	} else {
		sessionFilter = `
$session = $manager.GetCurrentSession()
if (-not $session) {
	Write-Error "No active media session"
	exit 1
}`
	}

	return fmt.Sprintf(`
Add-Type -AssemblyName Windows.Media.Control -ErrorAction SilentlyContinue
if (-not ('Windows.Media.Control.GlobalSystemMediaTransportControlsSessionManager' -as [type])) {
	Write-Error "GSMTC not available"
	exit 1
}

$manager = [Windows.Media.Control.GlobalSystemMediaTransportControlsSessionManager]::RequestAsync().GetAwaiter().GetResult()
$sessions = $manager.GetSessions()
%s

# Try to set repeat mode
try {
	$session.TryChangeRepeatEnabledAsync(%s).GetAwaiter().GetResult() | Out-Null
} catch {
	Write-Error ("Set repeat failed: " + $_.Exception.Message)
	exit 1
}
`, sessionFilter, enabled)
}

// GetSessions returns all active media sessions.
func (p *WindowsProvider) GetSessions() ([]MediaSession, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	sessionInfos, err := p.getSessionsInternal()
	if err != nil {
		return nil, err
	}

	sessions := make([]MediaSession, 0, len(sessionInfos))
	for _, si := range sessionInfos {
		sessions = append(sessions, MediaSession{
			ID:       si.ID,
			Name:     extractPlayerName(si.ID),
			Playing:  si.Playing,
			Priority: getPlayerPriority(si.ID),
		})
	}

	return sessions, nil
}

// GetState returns the current media playback state.
// Uses caching to reduce PowerShell spawn overhead.
func (p *WindowsProvider) GetState() (*MediaInfo, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Check if cache is still valid and session hasn't changed
	if p.cache != nil && time.Since(p.cacheTime) < p.cacheTTL && p.cacheSessionID == p.lastSessionID {
		// Return cached data, but update position estimate if playing
		cached := *p.cache
		if cached.Playing && cached.DurationMs > 0 {
			// Estimate current position based on elapsed time
			elapsed := time.Since(p.cacheTime).Milliseconds()
			cached.PositionMs = p.cache.PositionMs + elapsed
			if cached.PositionMs > cached.DurationMs {
				cached.PositionMs = cached.DurationMs
			}
		}
		return &cached, nil
	}

	// Find the best player to use
	sessionID, err := p.findPlayer()
	if err != nil {
		// On error, return stale cache if available
		if p.cache != nil {
			cached := *p.cache
			return &cached, nil
		}
		return nil, err
	}

	output, err := p.runPowerShell(p.getMediaInfoScript(sessionID))
	if err != nil {
		// On error, return stale cache if available
		if p.cache != nil {
			cached := *p.cache
			return &cached, nil
		}
		return nil, fmt.Errorf("failed to get media info: %w", err)
	}

	// Parse JSON output
	outputStr := strings.TrimSpace(string(output))
	if outputStr == "" {
		if p.cache != nil {
			cached := *p.cache
			return &cached, nil
		}
		return nil, fmt.Errorf("empty response from PowerShell")
	}

	var info gsmrtcMediaInfo
	if err := json.Unmarshal([]byte(outputStr), &info); err != nil {
		if p.cache != nil {
			cached := *p.cache
			return &cached, nil
		}
		return nil, fmt.Errorf("failed to parse media info JSON: %w (output: %s)", err, outputStr)
	}

	// Check for error in response
	if info.Title == "" && info.SessionID == "" {
		// Try to parse as error map
		var errResp struct {
			Error string `json:"Error"`
		}
		if json.Unmarshal([]byte(outputStr), &errResp) == nil && errResp.Error != "" {
			return nil, fmt.Errorf("%s", errResp.Error)
		}
		return nil, fmt.Errorf("no media session active")
	}

	// Cache the session ID
	p.lastSessionID = info.SessionID

	// Update cache
	result := &MediaInfo{
		Title:       info.Title,
		Artist:      info.Artist,
		Album:       info.Album,
		CoverURL:    "", // Use CoverBase64 for display instead
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

// GetCapabilities returns what operations the current player supports.
func (p *WindowsProvider) GetCapabilities() (*MediaCapabilities, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	sessionID, err := p.findPlayer()
	if err != nil {
		return &MediaCapabilities{}, err
	}

	output, err := p.runPowerShell(p.getCapabilitiesScript(sessionID))
	if err != nil {
		return &MediaCapabilities{}, fmt.Errorf("failed to get capabilities: %w", err)
	}

	outputStr := strings.TrimSpace(string(output))
	if outputStr == "" {
		return &MediaCapabilities{}, nil
	}

	var cap MediaCapabilities
	if err := json.Unmarshal([]byte(outputStr), &cap); err != nil {
		return &MediaCapabilities{}, fmt.Errorf("failed to parse capabilities JSON: %w", err)
	}

	return &cap, nil
}

// Play starts playback.
func (p *WindowsProvider) Play() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	sessionID, err := p.findPlayer()
	if err != nil {
		return fmt.Errorf("play failed: %w", err)
	}

	_, err = p.runPowerShell(p.controlPlaybackScript("Play", sessionID))
	if err != nil {
		return fmt.Errorf("play failed: %w", err)
	}
	return nil
}

// Pause pauses playback.
func (p *WindowsProvider) Pause() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	sessionID, err := p.findPlayer()
	if err != nil {
		return fmt.Errorf("pause failed: %w", err)
	}

	_, err = p.runPowerShell(p.controlPlaybackScript("Pause", sessionID))
	if err != nil {
		return fmt.Errorf("pause failed: %w", err)
	}
	return nil
}

// Next skips to the next track.
func (p *WindowsProvider) Next() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	sessionID, err := p.findPlayer()
	if err != nil {
		return fmt.Errorf("next failed: %w", err)
	}

	_, err = p.runPowerShell(p.controlPlaybackScript("Next", sessionID))
	if err != nil {
		return fmt.Errorf("next failed: %w", err)
	}
	return nil
}

// Previous skips to the previous track.
func (p *WindowsProvider) Previous() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	sessionID, err := p.findPlayer()
	if err != nil {
		return fmt.Errorf("previous failed: %w", err)
	}

	_, err = p.runPowerShell(p.controlPlaybackScript("Previous", sessionID))
	if err != nil {
		return fmt.Errorf("previous failed: %w", err)
	}
	return nil
}

// SeekTo jumps to the specified position in milliseconds.
func (p *WindowsProvider) SeekTo(positionMs int64) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	sessionID, err := p.findPlayer()
	if err != nil {
		return fmt.Errorf("seek failed: %w", err)
	}

	_, err = p.runPowerShell(p.seekScript(positionMs, sessionID))
	if err != nil {
		return fmt.Errorf("seek failed: %w", err)
	}
	return nil
}

// SetShuffle enables or disables shuffle mode.
func (p *WindowsProvider) SetShuffle(enabled bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	sessionID, err := p.findPlayer()
	if err != nil {
		return fmt.Errorf("set shuffle failed: %w", err)
	}

	_, err = p.runPowerShell(p.setShuffleScript(enabled, sessionID))
	if err != nil {
		return fmt.Errorf("set shuffle failed: %w", err)
	}
	return nil
}

// SetRepeat sets the repeat mode.
// mode: 0 = None, 1 = One, 2 = All
func (p *WindowsProvider) SetRepeat(mode int) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	sessionID, err := p.findPlayer()
	if err != nil {
		return fmt.Errorf("set repeat failed: %w", err)
	}

	_, err = p.runPowerShell(p.setRepeatScript(mode, sessionID))
	if err != nil {
		return fmt.Errorf("set repeat failed: %w", err)
	}
	return nil
}

// Close releases resources (no-op for PowerShell-based implementation).
func (p *WindowsProvider) Close() {
	// No persistent resources to clean up
}

// Ensure WindowsProvider implements MediaProvider at compile time.
var _ MediaProvider = (*WindowsProvider)(nil)
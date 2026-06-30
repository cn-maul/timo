//go:build linux

package media

import (
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/godbus/dbus/v5"
)

const (
	mprisPrefix      = "org.mpris.MediaPlayer2."
	mprisObject      = "/org/mpris/MediaPlayer2"
	mprisPlayerIface = "org.mpris.MediaPlayer2.Player"
	propertiesIface  = "org.freedesktop.DBus.Properties"
)

// LinuxProvider implements MediaProvider using MPRIS over D-Bus.
type LinuxProvider struct {
	conn         *dbus.Conn
	lastPlayer   string

	// Cached current track ID for SeekTo (MPRIS requires the actual track ID)
	currentTrackID dbus.ObjectPath

	// Signal subscription
	signalChan chan struct{}
	signalStop chan struct{}

	mu sync.Mutex
}

// NewLinuxProvider connects to the session bus.
func NewLinuxProvider() (*LinuxProvider, error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to session bus: %w", err)
	}
	return &LinuxProvider{conn: conn}, nil
}

// findPlayer returns the bus name of an active MPRIS player, preferring
// the last active player for stability when multiple players are running.
func (p *LinuxProvider) findPlayer() string {
	var names []string
	err := p.conn.BusObject().Call("org.freedesktop.DBus.ListNames", 0).Store(&names)
	if err != nil {
		return ""
	}
	var first string
	for _, name := range names {
		if strings.HasPrefix(name, mprisPrefix) && name != mprisPrefix+"d" {
			if first == "" {
				first = name
			}
			// Prefer the last active player for stability
			if name == p.lastPlayer {
				return name
			}
		}
	}
	p.lastPlayer = first
	return first
}

func (p *LinuxProvider) getPlayerObject() (dbus.BusObject, error) {
	busName := p.findPlayer()
	if busName == "" {
		return nil, fmt.Errorf("no MPRIS player found")
	}
	return p.conn.Object(busName, dbus.ObjectPath(mprisObject)), nil
}

func (p *LinuxProvider) GetState() (*MediaInfo, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	obj, err := p.getPlayerObject()
	if err != nil {
		return nil, err
	}

	info := &MediaInfo{}

	// Metadata
	metaVar, err := obj.GetProperty(mprisPlayerIface + ".Metadata")
	if err != nil {
		return nil, fmt.Errorf("failed to get metadata: %w", err)
	}
	metadata, ok := metaVar.Value().(map[string]dbus.Variant)
	if !ok {
		return nil, fmt.Errorf("unexpected metadata type")
	}

	if v, ok := metadata["mpris:trackid"]; ok {
		if trackID, ok := v.Value().(dbus.ObjectPath); ok && trackID != "" {
			p.currentTrackID = trackID
		}
	}

	if v, ok := metadata["xesam:title"]; ok {
		info.Title, _ = v.Value().(string)
	}
	if v, ok := metadata["xesam:album"]; ok {
		info.Album, _ = v.Value().(string)
	}
	if v, ok := metadata["xesam:artist"]; ok {
		if artists, ok := v.Value().([]string); ok && len(artists) > 0 {
			info.Artist = artists[0]
		} else if artist, ok := v.Value().(string); ok {
			info.Artist = artist
		}
	}
	if v, ok := metadata["mpris:artUrl"]; ok {
		info.CoverURL, _ = v.Value().(string)
	}
	if v, ok := metadata["mpris:length"]; ok {
		switch val := v.Value().(type) {
		case int64:
			info.DurationMs = val / 1000
		case uint64:
			info.DurationMs = int64(val / 1000)
		case float64:
			info.DurationMs = int64(val / 1000)
		}
	}

	// Playback status
	statusVar, err := obj.GetProperty(mprisPlayerIface + ".PlaybackStatus")
	if err == nil {
		if status, ok := statusVar.Value().(string); ok {
			info.Playing = status == "Playing"
		}
	} else {
		log.Printf("timo: failed to get playback status: %v", err)
	}

	// Position
	posVar, err := obj.GetProperty(mprisPlayerIface + ".Position")
	if err == nil {
		switch val := posVar.Value().(type) {
		case int64:
			info.PositionMs = val / 1000
		case uint64:
			info.PositionMs = int64(val / 1000)
		case float64:
			info.PositionMs = int64(val / 1000)
		}
	} else {
		log.Printf("timo: failed to get position: %v", err)
	}

	return info, nil
}

func (p *LinuxProvider) callPlayerMethod(method string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	obj, err := p.getPlayerObject()
	if err != nil {
		return err
	}
	return obj.Call(mprisPlayerIface+"."+method, 0).Err
}

func (p *LinuxProvider) Play() error     { return p.callPlayerMethod("Play") }
func (p *LinuxProvider) Pause() error    { return p.callPlayerMethod("Pause") }
func (p *LinuxProvider) Next() error     { return p.callPlayerMethod("Next") }
func (p *LinuxProvider) Previous() error { return p.callPlayerMethod("Previous") }

// SeekTo jumps to the specified position in milliseconds.
// Uses the cached track ID from the last GetState() call,
// as required by the MPRIS specification.
func (p *LinuxProvider) SeekTo(positionMs int64) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	obj, err := p.getPlayerObject()
	if err != nil {
		return err
	}
	// MPRIS uses microseconds for position
	positionUs := positionMs * 1000

	// Use the cached track ID; fall back to the MPRIS object path if unknown.
	trackID := p.currentTrackID
	if trackID == "" {
		trackID = dbus.ObjectPath(mprisObject)
	}

	return obj.Call(mprisPlayerIface+".SetPosition", 0, trackID, positionUs).Err
}

// SetShuffle enables or disables shuffle mode.
func (p *LinuxProvider) SetShuffle(enabled bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	obj, err := p.getPlayerObject()
	if err != nil {
		return err
	}
	return obj.Call(mprisPlayerIface+".SetShuffle", 0, enabled).Err
}

// SetRepeat sets the repeat mode.
// mode: 0 = None, 1 = One, 2 = All
func (p *LinuxProvider) SetRepeat(mode int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	obj, err := p.getPlayerObject()
	if err != nil {
		return err
	}
	// MPRIS RepeatMode: "None", "Track", "Playlist"
	var repeatMode string
	switch mode {
	case 0:
		repeatMode = "None"
	case 1:
		repeatMode = "Track"
	case 2:
		repeatMode = "Playlist"
	default:
		repeatMode = "None"
	}
	return obj.Call(mprisPlayerIface+".SetRepeatMode", 0, repeatMode).Err
}

// GetSessions returns all active MPRIS sessions.
func (p *LinuxProvider) GetSessions() ([]MediaSession, error) {
	var names []string
	err := p.conn.BusObject().Call("org.freedesktop.DBus.ListNames", 0).Store(&names)
	if err != nil {
		return nil, fmt.Errorf("failed to list bus names: %w", err)
	}

	var sessions []MediaSession
	for _, name := range names {
		if strings.HasPrefix(name, mprisPrefix) && name != mprisPrefix+"d" {
			obj := p.conn.Object(name, dbus.ObjectPath(mprisObject))

			// Get playback status
			playing := false
			statusVar, err := obj.GetProperty(mprisPlayerIface + ".PlaybackStatus")
			if err == nil {
				if status, ok := statusVar.Value().(string); ok {
					playing = status == "Playing"
				}
			}

			// Extract app name from MPRIS bus name
			appName := strings.TrimPrefix(name, mprisPrefix)

			sessions = append(sessions, MediaSession{
				ID:       name,
				Name:     appName,
				Playing:  playing,
				Priority: len(sessions),
			})
		}
	}
	return sessions, nil
}

// GetCapabilities returns what operations the current player supports.
func (p *LinuxProvider) GetCapabilities() (*MediaCapabilities, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	obj, err := p.getPlayerObject()
	if err != nil {
		return &MediaCapabilities{}, err
	}

	cap := &MediaCapabilities{}

	// Check capabilities via properties
	canSeekVar, err := obj.GetProperty(mprisPlayerIface + ".CanSeek")
	if err == nil {
		cap.CanSeek, _ = canSeekVar.Value().(bool)
	}

	canGoNextVar, err := obj.GetProperty(mprisPlayerIface + ".CanGoNext")
	if err == nil {
		cap.CanNext, _ = canGoNextVar.Value().(bool)
	}

	canGoPreviousVar, err := obj.GetProperty(mprisPlayerIface + ".CanGoPrevious")
	if err == nil {
		cap.CanPrevious, _ = canGoPreviousVar.Value().(bool)
	}

	canPlayVar, err := obj.GetProperty(mprisPlayerIface + ".CanPlay")
	if err == nil {
		cap.CanPlay, _ = canPlayVar.Value().(bool)
	}

	canPauseVar, err := obj.GetProperty(mprisPlayerIface + ".CanPause")
	if err == nil {
		cap.CanPause, _ = canPauseVar.Value().(bool)
	}

	// Shuffle and Repeat are typically supported by MPRIS players
	cap.CanShuffle = true
	cap.CanRepeat = true

	return cap, nil
}

// ── D-Bus PropertiesChanged signal subscription ──

// SubscribeSignals subscribes to MPRIS PropertiesChanged signals.
// Returns a channel that fires (unblocks) each time a relevant signal arrives.
// The caller should read from this channel and call GetState() to fetch the
// latest state, ensuring the polling loop has up-to-date data instantly.
func (p *LinuxProvider) SubscribeSignals() (<-chan struct{}, error) {
	p.mu.Lock()
	if p.signalChan != nil {
		p.mu.Unlock()
		return p.signalChan, nil
	}

	p.signalChan = make(chan struct{}, 16)
	p.signalStop = make(chan struct{})
	p.mu.Unlock()

	// Match rule: listen for PropertiesChanged on the MPRIS Player interface
	// from any MPRIS player bus name.
	matchRule := fmt.Sprintf(
		"type='signal',interface='%s',path='%s'",
		propertiesIface, mprisObject,
	)
	busObj := p.conn.BusObject()
	if err := busObj.Call("org.freedesktop.DBus.AddMatch", 0, matchRule).Err; err != nil {
		p.mu.Lock()
		p.signalChan = nil
		p.signalStop = nil
		p.mu.Unlock()
		return nil, fmt.Errorf("failed to add D-Bus match: %w", err)
	}

	// Register to receive all signals
	signalCh := make(chan *dbus.Signal, 16)
	p.conn.Signal(signalCh)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("timo: media signal handler panic recovered: %v", r)
			}
		}()
		defer func() {
			p.conn.RemoveSignal(signalCh)
			// Try to remove the match (best-effort)
			_ = busObj.Call("org.freedesktop.DBus.RemoveMatch", 0, matchRule).Err
		}()

		for {
			select {
			case sig := <-signalCh:
				if sig == nil {
					continue
				}
				// We only care about PropertiesChanged on the MPRIS player interface
				if sig.Name != propertiesIface+".PropertiesChanged" {
					continue
				}
				// Signal body: [iface_name (string), changed_properties (map), invalidated ([]string)]
				if len(sig.Body) < 1 {
					continue
				}
				ifaceName, ok := sig.Body[0].(string)
				if !ok || ifaceName != mprisPlayerIface {
					continue
				}
				// Notify the channel (non-blocking, drop if full)
				select {
				case p.signalChan <- struct{}{}:
				default:
				}

			case <-p.signalStop:
				return
			}
		}
	}()

	log.Printf("timo: subscribed to MPRIS PropertiesChanged signals on %s", mprisObject)
	return p.signalChan, nil
}

// UnsubscribeSignals stops the signal listener goroutine and removes D-Bus match.
func (p *LinuxProvider) UnsubscribeSignals() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.signalStop == nil {
		return
	}
	select {
	case <-p.signalStop:
		// already closed
	default:
		close(p.signalStop)
	}
	p.signalChan = nil
}

func (p *LinuxProvider) Close() {
	p.UnsubscribeSignals()
	if p.conn != nil {
		p.conn.Close()
	}
}

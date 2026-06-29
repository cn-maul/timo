//go:build linux

package media

import (
	"fmt"
	"log"
	"strings"

	"github.com/godbus/dbus/v5"
)

const (
	mprisPrefix      = "org.mpris.MediaPlayer2."
	mprisObject      = "/org/mpris/MediaPlayer2"
	mprisPlayerIface = "org.mpris.MediaPlayer2.Player"
)

// LinuxProvider implements MediaProvider using MPRIS over D-Bus.
type LinuxProvider struct {
	conn       *dbus.Conn
	lastPlayer string
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
func (p *LinuxProvider) SeekTo(positionMs int64) error {
	obj, err := p.getPlayerObject()
	if err != nil {
		return err
	}
	// MPRIS uses microseconds for position
	positionUs := positionMs * 1000
	return obj.Call(mprisPlayerIface+".SetPosition", 0, dbus.ObjectPath("/org/mpris/MediaPlayer2"), positionUs).Err
}

// SetShuffle enables or disables shuffle mode.
func (p *LinuxProvider) SetShuffle(enabled bool) error {
	obj, err := p.getPlayerObject()
	if err != nil {
		return err
	}
	return obj.Call(mprisPlayerIface+".SetShuffle", 0, enabled).Err
}

// SetRepeat sets the repeat mode.
// mode: 0 = None, 1 = One, 2 = All
func (p *LinuxProvider) SetRepeat(mode int) error {
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

func (p *LinuxProvider) Close() {
	if p.conn != nil {
		p.conn.Close()
	}
}

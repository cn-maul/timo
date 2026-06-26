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
	conn *dbus.Conn
}

// NewLinuxProvider connects to the session bus.
func NewLinuxProvider() (*LinuxProvider, error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to session bus: %w", err)
	}
	return &LinuxProvider{conn: conn}, nil
}

// findPlayer returns the bus name of the first active MPRIS player.
func (p *LinuxProvider) findPlayer() string {
	var names []string
	err := p.conn.BusObject().Call("org.freedesktop.DBus.ListNames", 0).Store(&names)
	if err != nil {
		return ""
	}
	for _, name := range names {
		if strings.HasPrefix(name, mprisPrefix) && name != mprisPrefix+"d" {
			// Exclude "d" — the KDE Plasma media controller integration
			return name
		}
	}
	return ""
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

func (p *LinuxProvider) Close() {
	if p.conn != nil {
		p.conn.Close()
	}
}

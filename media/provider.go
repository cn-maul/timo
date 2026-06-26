package media

// MediaInfo holds the current media playback state.
type MediaInfo struct {
	Title      string `json:"title"`
	Artist     string `json:"artist"`
	Album      string `json:"album"`
	CoverURL   string `json:"coverUrl"`
	DurationMs int64  `json:"durationMs"`
	PositionMs int64  `json:"positionMs"`
	Playing    bool   `json:"playing"`
}

// MediaProvider is the interface for platform-specific media integration.
type MediaProvider interface {
	// GetState returns the current media playback info.
	GetState() (*MediaInfo, error)

	// Playback controls.
	Play() error
	Pause() error
	Next() error
	Previous() error

	// Close releases any resources (D-Bus connections, etc.).
	Close()
}

package media

// MediaCapabilities describes what operations the current media player supports.
type MediaCapabilities struct {
	CanPlay     bool `json:"canPlay"`
	CanPause    bool `json:"canPause"`
	CanNext     bool `json:"canNext"`
	CanPrevious bool `json:"canPrevious"`
	CanSeek     bool `json:"canSeek"`
	CanShuffle  bool `json:"canShuffle"`
	CanRepeat   bool `json:"canRepeat"`
}

// MediaInfo holds the current media playback state.
type MediaInfo struct {
	Title       string `json:"title"`
	Artist      string `json:"artist"`
	Album       string `json:"album"`
	CoverURL    string `json:"coverUrl"`
	CoverBase64 string `json:"coverBase64,omitempty"`
	DurationMs  int64  `json:"durationMs"`
	PositionMs  int64  `json:"positionMs"`
	Playing     bool   `json:"playing"`
}

// MediaSession represents a single media session with its metadata.
type MediaSession struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Playing  bool   `json:"playing"`
	Priority int    `json:"priority"` // Lower = higher priority
}

// MediaProvider is the interface for platform-specific media integration.
type MediaProvider interface {
	// GetState returns the current media playback info.
	GetState() (*MediaInfo, error)

	// GetSessions returns all active media sessions.
	GetSessions() ([]MediaSession, error)

	// GetCapabilities returns what operations the current player supports.
	GetCapabilities() (*MediaCapabilities, error)

	// Playback controls.
	Play() error
	Pause() error
	Next() error
	Previous() error

	// Seek jumps to the specified position in milliseconds.
	// Returns an error if the media doesn't support seeking.
	SeekTo(positionMs int64) error

	// SetShuffle enables or disables shuffle mode.
	// Returns an error if the media doesn't support shuffle.
	SetShuffle(enabled bool) error

	// SetRepeat sets the repeat mode.
	// Returns an error if the media doesn't support repeat.
	SetRepeat(mode int) error

	// Close releases any resources (D-Bus connections, etc.).
	Close()
}

//go:build windows

package media

// NewProvider returns the platform-specific MediaProvider.
func NewProvider() (MediaProvider, error) {
	return NewWindowsProvider()
}

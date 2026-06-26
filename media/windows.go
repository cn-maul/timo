//go:build windows

// WindowsProvider is the Windows implementation of MediaProvider.
//
// TODO: The planned approach is a PowerShell GSMTC (Global System Media Transport
// Controls) bridge — invoking Get-TransportControls via a spawned PowerShell
// process to read/interact with the active media session. This avoids CGo and
// extra DLL dependencies while covering all UWP-compatible players.
package media

import "fmt"

// WindowsProvider implements MediaProvider using GSMTC via PowerShell.
type WindowsProvider struct{}

func NewWindowsProvider() (*WindowsProvider, error) {
	return &WindowsProvider{}, nil
}

func (p *WindowsProvider) GetState() (*MediaInfo, error) {
	return nil, fmt.Errorf("windows media provider: not yet implemented (see TODO in media/windows.go)")
}

func (p *WindowsProvider) Play() error     { return fmt.Errorf("windows media provider: not yet implemented (see TODO in media/windows.go)") }
func (p *WindowsProvider) Pause() error    { return fmt.Errorf("windows media provider: not yet implemented (see TODO in media/windows.go)") }
func (p *WindowsProvider) Next() error     { return fmt.Errorf("windows media provider: not yet implemented (see TODO in media/windows.go)") }
func (p *WindowsProvider) Previous() error { return fmt.Errorf("windows media provider: not yet implemented (see TODO in media/windows.go)") }
func (p *WindowsProvider) Close()            {}

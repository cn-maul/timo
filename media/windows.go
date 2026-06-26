//go:build windows

package media

import "fmt"

// WindowsProvider implements MediaProvider using GSMTC via PowerShell.
type WindowsProvider struct{}

func NewWindowsProvider() (*WindowsProvider, error) {
	return &WindowsProvider{}, nil
}

func (p *WindowsProvider) GetState() (*MediaInfo, error) {
	// TODO: implement via PowerShell GSMTC bridge
	return nil, fmt.Errorf("Windows media provider not yet implemented")
}

func (p *WindowsProvider) Play() error       { return fmt.Errorf("not implemented") }
func (p *WindowsProvider) Pause() error      { return fmt.Errorf("not implemented") }
func (p *WindowsProvider) Next() error       { return fmt.Errorf("not implemented") }
func (p *WindowsProvider) Previous() error   { return fmt.Errorf("not implemented") }
func (p *WindowsProvider) Close()            {}

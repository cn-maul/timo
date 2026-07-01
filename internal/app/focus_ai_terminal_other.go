//go:build !linux

package app

// focusAITerminal is a no-op on non-Linux platforms.
func focusAITerminal(response string) {
	_ = response
}

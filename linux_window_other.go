//go:build !linux

package main

import "unsafe"

// configureDockWindow is a no-op on non-Linux platforms.
// On Windows, use HiddenOnTaskbar in WindowsWindow options.
func configureDockWindow(_ unsafe.Pointer) {}

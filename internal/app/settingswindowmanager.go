package app

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// SettingsWindowManager manages the standalone settings window lifecycle.
type SettingsWindowManager struct {
	app    *application.App
	win    WindowController
	window *application.WebviewWindow
}

// NewSettingsWindowManager creates a new settings window manager.
func NewSettingsWindowManager(app *application.App, win WindowController) *SettingsWindowManager {
	return &SettingsWindowManager{app: app, win: win}
}

// screenCenter computes on-screen pixel coords to center a box of (w,h) on the
// screen that the main notch window currently sits on.
func (m *SettingsWindowManager) screenCenter(w, h int) (x, y int, ok bool) {
	if m.win == nil {
		return 0, 0, false
	}
	screen, err := m.win.GetScreen()
	if err != nil || screen == nil {
		return 0, 0, false
	}
	sw, sh := screen.Size.Width, screen.Size.Height
	if sw <= 0 || sh <= 0 {
		return 0, 0, false
	}
	return (sw - w) / 2, (sh - h) / 2, true
}

// Open opens or shows the settings window. If the window was already created
// but hidden, it is shown again. Otherwise a new window is created.
func (m *SettingsWindowManager) Open() {
	if m.app == nil {
		return
	}

	// Reuse the existing window if it is still alive.
	if m.window != nil && !m.isDead() {
		m.window.Show()
		m.window.UnMinimise()
		m.window.Focus()
		return
	}

	const (
		w = 640
		h = 500
	)

	opts := application.WebviewWindowOptions{
		Title:            "Timo 设置",
		Width:            w,
		Height:           h,
		MinWidth:         w,
		MinHeight:        400,
		Frameless:        true,
		AlwaysOnTop:      false,
		BackgroundType:   application.BackgroundTypeSolid,
		BackgroundColour: application.NewRGBA(0, 0, 0, 0),
		URL:              "/?settings=1",
		Linux: application.LinuxWindow{
			WindowIsTranslucent: false,
		},
	}
	if x, y, ok := m.screenCenter(w, h); ok {
		opts.InitialPosition = application.WindowXY
		opts.X = x
		opts.Y = y
	} else {
		opts.InitialPosition = application.WindowCentered
	}

	m.window = m.app.Window.NewWithOptions(opts)

	// Intercept the window's close (the X button / WM delete) and just hide it,
	// so the window can be reopened later without recreating it.
	m.window.OnWindowEvent(events.Common.WindowClosing, func(event *application.WindowEvent) {
		if m.window != nil {
			m.window.Hide()
		}
	})
}

// Close hides the settings window. The window is kept alive to be reopened.
func (m *SettingsWindowManager) Close() {
	if m.window != nil {
		m.window.Hide()
	}
}

// isDead reports whether the cached settings window has been torn down.
func (m *SettingsWindowManager) isDead() bool {
	if m.window == nil {
		return true
	}
	w, ok := m.app.Window.GetByID(m.window.ID())
	return !ok || w == nil
}

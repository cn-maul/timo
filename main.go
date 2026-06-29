package main

import (
	"embed"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"timo/media"
)

//go:embed all:frontend/dist
var assets embed.FS

var mainApp *application.App
var mainWindow *application.WebviewWindow

// settingsWindow is a separate window for the settings UI, kept alive across
// open/close cycles (hidden rather than destroyed) so it can be reopened. It
// lives on the main thread just like mainWindow.
var settingsWindow *application.WebviewWindow

// screenCenter computes on-screen pixel coords to center a box of (w,h) on the
// screen that the main notch window currently sits on. Falls back to the
// window's own screen when available.
func screenCenter(w, h int) (x, y int, ok bool) {
	var sw, sh int
	screen, err := mainWindow.GetScreen()
	if err == nil && screen != nil {
		sw, sh = screen.Size.Width, screen.Size.Height
	}
	if sw <= 0 || sh <= 0 {
		return 0, 0, false
	}
	return (sw - w) / 2, (sh - h) / 2, true
}

// openSettings opens a dedicated, centered settings window. The notch window
// is left untouched. The window is created once and then shown/hidden on
// subsequent opens.
func openSettings() {
	if mainApp == nil {
		return
	}

	// Reuse the existing window if it is still alive.
	if settingsWindow != nil && !settingsWindowIsDead() {
		settingsWindow.Show()
		settingsWindow.UnMinimise()
		settingsWindow.Focus()
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
	if x, y, ok := screenCenter(w, h); ok {
		opts.InitialPosition = application.WindowXY
		opts.X = x
		opts.Y = y
	} else {
		opts.InitialPosition = application.WindowCentered
	}

	settingsWindow = mainApp.Window.NewWithOptions(opts)

	// Intercept the window's close (the X button / WM delete) and just hide it,
	// so the window can be reopened later without recreating it. Emitting
	// close-settings also lets the frontend reset transient state.
	settingsWindow.OnWindowEvent(events.Common.WindowClosing, func(event *application.WindowEvent) {
		if settingsWindow != nil {
			settingsWindow.Hide()
		}
	})
}

// settingsWindowIsDead reports whether the cached settings window has been
// torn down and must be recreated. Wails exposes no public IsDestroyed, so we
// rely on whether it is still registered in the window manager (by ID).
func settingsWindowIsDead() bool {
	if settingsWindow == nil {
		return true
	}
	w, ok := mainApp.Window.GetByID(settingsWindow.ID())
	return !ok || w == nil
}

func init() {
	application.RegisterEvent[*media.MediaInfo]("media-update")
	application.RegisterEvent[*Notification]("notification")
	application.RegisterEvent[*SystemStats]("sys-stats")
	application.RegisterEvent[*TimoSettings]("settings-updated")
}


func main() {
	// CLI mode
	if IsCLI() {
		if len(os.Args) > 1 && os.Args[1] == "setup" {
			RunSetup()
		} else if len(os.Args) > 1 && os.Args[1] == "setup-reasonix" {
			RunSetupReasonix()
		} else {
			RunCLI()
		}
		return
	}

	// Auto-inject Claude Code and Reasonix hooks on first run
	// Commented out — users now control hook injection from Settings UI
	// AutoSetupHooks()
	// AutoSetupReasonixHooks()

	// GUI mode: start the Wails app
	mainApp = application.New(application.Options{
		Name:        "Timo",
		Description: "Dynamic Island for desktop",
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	// Media provider
	provider, err := media.NewProvider()
	if err != nil {
		log.Printf("timo: media provider init failed (%v) — media playback display will be unavailable", err)
	}

	var poller *media.Poller
	var mediaSvc *MediaService
	if provider != nil {
		emitter := func(info *media.MediaInfo) {
			mainApp.Event.Emit("media-update", info)
		}
		poller = media.NewPoller(provider, emitter, 1*time.Second)
		mediaSvc = NewMediaService(poller)
		mainApp.RegisterService(application.NewService(mediaSvc))
	}

	// Notification server (for Claude Code / Reasonix hooks)
	notifServer := NewNotifyServer(func(n Notification) {
		mainApp.Event.Emit("notification", &n)
	})
	if err := notifServer.Start(); err != nil {
		log.Printf("Warning: notification server failed: %v", err)
	} else {
		log.Printf("Notification socket: %s", GetSocketPath())
	}

	// Process monitor (Claude Code + Reasonix)
	processMonitor := NewProcessMonitor([]string{"claude", "reasonix"}, func(n Notification) {
		mainApp.Event.Emit("notification", &n)
	})
	processMonitor.Start()

	// Settings service
	settingsService := NewSettingsService()
	mainApp.RegisterService(application.NewService(settingsService))
	registerSettingsEventHandlers(mainApp, settingsService)

	// System stats poller (CPU + Memory)
	sysPoller := NewSystemPoller(func(stats SystemStats) {
		mainApp.Event.Emit("sys-stats", &stats)
	})

	mainWindow = mainApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "Timo",
		Width:            600,
		Height:           64,
		Frameless:        true,
		AlwaysOnTop:      true,
		BackgroundType:   application.BackgroundTypeTransparent,
		BackgroundColour: application.NewRGBA(0, 0, 0, 0),
		InitialPosition:  application.WindowCentered,
		URL:              "/",
		MinimiseButtonState: application.ButtonHidden,
		MaximiseButtonState: application.ButtonHidden,
		CloseButtonState:    application.ButtonHidden,
		Linux: application.LinuxWindow{
			WindowIsTranslucent: true,
		},
		Windows: application.WindowsWindow{
			DisableFramelessWindowDecorations: true,
			HiddenOnTaskbar:                   true,
		},
	})

	mainWindow.SetBackgroundColour(application.NewRGBA(0, 0, 0, 0))

	// Guard so pollers are only started once (WindowShow can fire multiple times)
	var startOnce sync.Once

	mainWindow.OnWindowEvent(events.Common.WindowShow, func(event *application.WindowEvent) {
		screen, err := mainWindow.GetScreen()
		if err == nil && screen != nil {
			sw := screen.Size.Width
			notchW := sw / 3
			windowW := notchW + 16
			mainWindow.SetSize(windowW, 64)
			x := (sw - windowW) / 2
			mainWindow.SetPosition(x, 0)
		}
		// Configure as dock window: skip taskbar, always on top
		configureDockWindow(mainWindow.NativeWindow())
		startOnce.Do(func() {
			if poller != nil {
				poller.Start()
			}
			sysPoller.Start()
		})
	})

	// System tray (right-click menu in notification area)
	var tray *application.SystemTray
	tray = setupSystemTray(
		mainApp,
		func() {
			// toggle window visibility
			if mainWindow.IsMinimised() || !mainWindow.IsVisible() {
				mainWindow.Show()
				mainWindow.Focus()
			} else {
				mainWindow.Minimise()
			}
		},
		func() {
			// open settings — resizes the main notch window to 1/3 screen
			openSettings()
		},
		func() {
			// quit
			mainApp.Quit()
		},
	)

	// Keep tray menu in sync when settings change
	mainApp.Event.On("settings-updated", func(event *application.CustomEvent) {
		if tray != nil {
			if settings, ok := event.Data.(*TimoSettings); ok {
				UpdateTrayStatus(tray, *settings)
			}
		}
	})

	// Global/app-level hotkeys
	initialSettings, _ := LoadSettings()
	hotkeyManager, hotkeyErr := NewGlobalHotkeyManager()
	if hotkeyErr != nil {
		log.Printf("timo: hotkey manager unavailable: %v", hotkeyErr)
	}
	if hotkeyManager != nil {
		// Toggle window hotkey
		if initialSettings.Hotkeys.Enabled && initialSettings.Hotkeys.ToggleWindow != "" {
			hotkeyManager.Register(initialSettings.Hotkeys.ToggleWindow, func() {
				if mainWindow.IsMinimised() || !mainWindow.IsVisible() {
					mainWindow.Show()
					mainWindow.Focus()
				} else {
					mainWindow.Minimise()
				}
			})
		}
		// Toggle media hotkey
		if initialSettings.Hotkeys.Enabled && initialSettings.Hotkeys.ToggleMedia != "" && mediaSvc != nil {
			hotkeyManager.Register(initialSettings.Hotkeys.ToggleMedia, func() {
				// Simple toggle: try play, if fails try pause
				if err := mediaSvc.Play(); err != nil {
					mediaSvc.Pause()
				}
			})
		}
		hotkeyManager.logStatus()
		// Wire app-level keybindings (work when window has focus)
		setupAppHotkeys(mainApp, &initialSettings, hotkeyManager)
	}

	// Open settings from tray (after menu rebuild) and other sources
	mainApp.Event.On("open-settings", func(event *application.CustomEvent) {
		openSettings()
	})

	// Close settings window (frontend "✕" button) — hide, keep it openable again.
	mainApp.Event.On("close-settings", func(event *application.CustomEvent) {
		if settingsWindow != nil {
			settingsWindow.Hide()
		}
	})

	// Hooks status query
	mainApp.Event.On("get-hooks-status", func(event *application.CustomEvent) {
		status := getHooksStatus()
		mainApp.Event.Emit("hooks-status", status)
	})

	// Inject hooks
	mainApp.Event.On("inject-hook", func(event *application.CustomEvent) {
		tool, _ := event.Data.(string)
		var msg string
		switch tool {
		case "claude":
			if _, err := setupHooks(false); err != nil {
				msg = "❌ Claude Code Hook 注入失败: " + err.Error()
			} else {
				msg = "✓ Claude Code Hook 已注入"
			}
		case "reasonix":
			if _, err := setupReasonixHooks(false); err != nil {
				msg = "❌ Reasonix Hook 注入失败: " + err.Error()
			} else {
				msg = "✓ Reasonix Hook 已注入"
			}
		case "all":
			var errs []string
			if _, err := setupHooks(false); err != nil {
				errs = append(errs, "Claude: "+err.Error())
			}
			if _, err := setupReasonixHooks(false); err != nil {
				errs = append(errs, "Reasonix: "+err.Error())
			}
			if len(errs) > 0 {
				msg = "❌ 部分注入失败: " + strings.Join(errs, "; ")
			} else {
				msg = "✓ 全部 Hook 已注入"
			}
		}
		mainApp.Event.Emit("hooks-feedback", msg)
		// Refresh status
		status := getHooksStatus()
		mainApp.Event.Emit("hooks-status", status)
	})

	// Remove hooks
	mainApp.Event.On("remove-hook", func(event *application.CustomEvent) {
		tool, _ := event.Data.(string)
		var msg string
		switch tool {
		case "claude":
			if err := removeHooks(".claude"); err != nil {
				msg = "❌ 移除失败: " + err.Error()
			} else {
				msg = "✓ Claude Code Hook 已移除"
			}
		case "reasonix":
			if err := removeHooks(".reasonix"); err != nil {
				msg = "❌ 移除失败: " + err.Error()
			} else {
				msg = "✓ Reasonix Hook 已移除"
			}
		}
		mainApp.Event.Emit("hooks-feedback", msg)
		status := getHooksStatus()
		mainApp.Event.Emit("hooks-status", status)
	})

	// Defer cleanup so it runs even on panic
	defer func() {
		notifServer.Stop()
		processMonitor.Stop()
		sysPoller.Stop()
		if poller != nil {
			poller.Stop()
		}
		if provider != nil {
			provider.Close()
		}
	}()

	if err := mainApp.Run(); err != nil {
		log.Fatal(err)
	}
}

// setupAppHotkeys registers Wails app-level keybindings that work when
// the Timo window has focus. For system-wide hotkeys, the GlobalHotkeyManager
// handles registration via D-Bus/X11.
func setupAppHotkeys(app *application.App, settings *TimoSettings, hkm *GlobalHotkeyManager) {
	if settings == nil || !settings.Hotkeys.Enabled {
		return
	}

	// Toggle window
	if settings.Hotkeys.ToggleWindow != "" {
		app.KeyBinding.Add(settings.Hotkeys.ToggleWindow, func(w application.Window) {
			if hkm != nil {
				hkm.Trigger(settings.Hotkeys.ToggleWindow)
			}
		})
	}

	// Toggle media
	if settings.Hotkeys.ToggleMedia != "" {
		app.KeyBinding.Add(settings.Hotkeys.ToggleMedia, func(w application.Window) {
			if hkm != nil {
				hkm.Trigger(settings.Hotkeys.ToggleMedia)
			}
		})
	}
}

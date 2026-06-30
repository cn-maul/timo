package app

import (
	"embed"
	"log"
	"os"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"timo/internal/media"
)

// Run is the application entry point called from main.go.
func Run(assets embed.FS) {
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

	// GUI mode: start the Wails app
	wailsApp := application.New(application.Options{
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
			wailsApp.Event.Emit("media-update", info)
		}
		poller = media.NewPoller(provider, emitter, 1*time.Second)

		// Subscribe to D-Bus PropertiesChanged signals for instant media updates
		if sp, ok := provider.(interface {
			SubscribeSignals() (<-chan struct{}, error)
			UnsubscribeSignals()
		}); ok {
			if sigCh, err := sp.SubscribeSignals(); err == nil {
				poller.SetSignalSource(sigCh)
				log.Printf("timo: subscribed to MPRIS PropertiesChanged signals")
			} else {
				log.Printf("timo: media signal subscription failed (falling back to polling): %v", err)
			}
		}

		mediaSvc = NewMediaService(poller)
		wailsApp.RegisterService(application.NewService(mediaSvc))
	}

	// Notification server (for Claude Code / Reasonix hooks)
	notifServer := NewNotifyServer(func(n Notification) {
		wailsApp.Event.Emit("notification", &n)
	})
	if err := notifServer.Start(); err != nil {
		log.Printf("Warning: notification server failed: %v", err)
	} else {
		log.Printf("Notification socket: %s", GetSocketPath())
	}

	// Process monitor (Claude Code + Reasonix)
	processMonitor := NewProcessMonitor([]string{"claude", "reasonix"}, func(n Notification) {
		wailsApp.Event.Emit("notification", &n)
	})
	processMonitor.Start()

	// Settings service (EventBus injected)
	settingsService := NewSettingsService(wailsApp.Event)
	wailsApp.RegisterService(application.NewService(settingsService))
	registerSettingsEventHandlers(wailsApp, settingsService)

	// System stats poller (CPU + Memory)
	sysPoller := NewSystemPoller(func(stats SystemStats) {
		wailsApp.Event.Emit("sys-stats", &stats)
	})

	// Main notched window
	win := wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
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

	win.SetBackgroundColour(application.NewRGBA(0, 0, 0, 0))

	// Settings window manager (encapsulates settings window lifecycle)
	settingsWinMgr := NewSettingsWindowManager(wailsApp, win)

	// Guard so pollers are only started once (WindowShow can fire multiple times)
	var startOnce sync.Once

	win.OnWindowEvent(events.Common.WindowShow, func(event *application.WindowEvent) {
		screen, err := win.GetScreen()
		if err == nil && screen != nil {
			sw := screen.Size.Width
			notchW := sw / 3
			windowW := notchW + 16
			win.SetSize(windowW, 64)
			x := (sw - windowW) / 2
			win.SetPosition(x, 0)
		}
		// Configure as dock window: skip taskbar, always on top
		configureDockWindow(win.NativeWindow())
		startOnce.Do(func() {
			if poller != nil {
				poller.Start()
			}
			sysPoller.Start()
		})
	})

	// System tray (right-click menu in notification area)
	tray := setupSystemTray(
		wailsApp,
		wailsApp.Event,
		win,
		func() {
			// toggle window visibility
			if win.IsMinimised() || !win.IsVisible() {
				win.Show()
				win.Focus()
			} else {
				win.Minimise()
			}
		},
		func() {
			// open settings
			settingsWinMgr.Open()
		},
		func() {
			// quit
			wailsApp.Quit()
		},
	)

	// Register all event handlers (moved to events.go)
	registerAllEventHandlers(wailsApp, tray, wailsApp.Event, win, func() { wailsApp.Quit() }, settingsWinMgr)

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
				if win.IsMinimised() || !win.IsVisible() {
					win.Show()
					win.Focus()
				} else {
					win.Minimise()
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
		setupAppHotkeys(wailsApp, &initialSettings, hotkeyManager)
	}

	// Defer cleanup so it runs even on panic
	cleanup := func() {
		notifServer.Stop()
		processMonitor.Stop()
		sysPoller.Stop()
		if poller != nil {
			poller.Stop()
		}
		if provider != nil {
			provider.Close()
		}
	}
	defer cleanup()

	if err := wailsApp.Run(); err != nil {
		log.Printf("timo: %v", err)
		cleanup()
		os.Exit(1)
	}
}

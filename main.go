//go:build linux

package main

import (
	"embed"
	"log"
	"os"
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

func init() {
	application.RegisterEvent[*media.MediaInfo]("media-update")
	application.RegisterEvent[*Notification]("notification")
	application.RegisterEvent[*SystemStats]("sys-stats")
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
	AutoSetupHooks()
	AutoSetupReasonixHooks()

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
	if provider != nil {
		emitter := func(info *media.MediaInfo) {
			mainApp.Event.Emit("media-update", info)
		}
		poller = media.NewPoller(provider, emitter, 1*time.Second)
		mainApp.RegisterService(application.NewService(NewMediaService(poller)))
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
		DisableResize:    true,
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

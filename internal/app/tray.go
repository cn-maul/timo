//go:build linux || windows

package app

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed logo.png
var trayIconFS embed.FS

// buildTrayMenu builds the tray menu with theme submenu, show/hide, settings, and quit items.
// It is used by both setupSystemTray and UpdateTrayStatus to avoid code duplication.
// The callbacks allow each caller to provide its own behavior for each action.
func buildTrayMenu(
	tray *application.SystemTray,
	settings TimoSettings,
	bus EventBus,
	win WindowController,
	toggleWindow func(),
	openSettings func(),
	quit func(),
) {
	menu := application.NewMenu()

	menu.Add("显示/隐藏 Timo").OnClick(func(ctx *application.Context) {
		toggleWindow()
	})

	menu.Add("设置").OnClick(func(ctx *application.Context) {
		openSettings()
	})

	menu.AddSeparator()

	// Theme submenu with checkmarks based on current settings
	themeMenu := menu.AddSubmenu("主题")

	darkItem := themeMenu.Add("深色")
	if settings.Theme == "dark" {
		darkItem.SetChecked(true)
	}
	darkItem.OnClick(func(ctx *application.Context) {
		s, _ := LoadSettings()
		s.Theme = "dark"
		SaveSettings(s)
		if bus != nil {
			bus.Emit("settings-loaded", &s)
		}
		UpdateTrayStatus(tray, s, bus, win, quit)
	})

	lightItem := themeMenu.Add("浅色")
	if settings.Theme == "light" {
		lightItem.SetChecked(true)
	}
	lightItem.OnClick(func(ctx *application.Context) {
		s, _ := LoadSettings()
		s.Theme = "light"
		SaveSettings(s)
		if bus != nil {
			bus.Emit("settings-loaded", &s)
		}
		UpdateTrayStatus(tray, s, bus, win, quit)
	})

	frostedItem := themeMenu.Add("磨砂")
	if settings.Theme == "frosted" {
		frostedItem.SetChecked(true)
	}
	frostedItem.OnClick(func(ctx *application.Context) {
		s, _ := LoadSettings()
		s.Theme = "frosted"
		SaveSettings(s)
		if bus != nil {
			bus.Emit("settings-loaded", &s)
		}
		UpdateTrayStatus(tray, s, bus, win, quit)
	})

	menu.AddSeparator()

	menu.Add("退出").OnClick(func(ctx *application.Context) {
		quit()
	})

	tray.SetMenu(menu)
}

func setupSystemTray(
	app *application.App,
	bus EventBus,
	win WindowController,
	toggleWindow func(),
	openSettings func(),
	quit func(),
) *application.SystemTray {
	tray := app.SystemTray.New()
	tray.SetTooltip("Timo")

	// Load tray icon
	iconData, err := trayIconFS.ReadFile("logo.png")
	if err != nil {
		log.Printf("timo: failed to load tray icon: %v", err)
	} else {
		tray.SetIcon(iconData)
	}

	buildTrayMenu(tray, TimoSettings{}, bus, win, toggleWindow, openSettings, quit)

	log.Printf("timo: system tray initialized")
	return tray
}

// UpdateTrayStatus rebuilds the tray menu to reflect current settings.
// Called when settings change.
func UpdateTrayStatus(tray *application.SystemTray, settings TimoSettings, bus EventBus, win WindowController, quit func()) {
	buildTrayMenu(tray, settings, bus, win,
		// toggleWindow: show/minimise via win
		func() {
			if win != nil {
				if win.IsMinimised() || !win.IsVisible() {
					win.Show()
				} else {
					win.Minimise()
				}
			}
		},
		// openSettings: emit event
		func() {
			if bus != nil {
				bus.Emit("open-settings", nil)
			}
		},
		// quit: use quit callback
		func() {
			if quit != nil {
				quit()
			}
		},
	)
}

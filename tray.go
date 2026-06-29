//go:build linux || windows

package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed logo.png
var trayIconFS embed.FS

func setupSystemTray(
	app *application.App,
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

	// Build initial menu
	menu := application.NewMenu()

	menu.Add("显示/隐藏 Timo").OnClick(func(ctx *application.Context) {
		toggleWindow()
	})

	menu.Add("设置").OnClick(func(ctx *application.Context) {
		openSettings()
	})

	menu.AddSeparator()

	// Theme submenu (clickable)
	themeMenu := menu.AddSubmenu("主题")
	themeMenu.Add("深色").OnClick(func(ctx *application.Context) {
		settings, _ := LoadSettings()
		settings.Theme = "dark"
		SaveSettings(settings)
		if mainApp != nil {
			mainApp.Event.Emit("settings-loaded", &settings)
		}
		UpdateTrayStatus(tray, settings)
	})
	themeMenu.Add("浅色").OnClick(func(ctx *application.Context) {
		settings, _ := LoadSettings()
		settings.Theme = "light"
		SaveSettings(settings)
		if mainApp != nil {
			mainApp.Event.Emit("settings-loaded", &settings)
		}
		UpdateTrayStatus(tray, settings)
	})
	themeMenu.Add("磨砂").OnClick(func(ctx *application.Context) {
		settings, _ := LoadSettings()
		settings.Theme = "frosted"
		SaveSettings(settings)
		if mainApp != nil {
			mainApp.Event.Emit("settings-loaded", &settings)
		}
		UpdateTrayStatus(tray, settings)
	})

	menu.AddSeparator()

	menu.Add("退出").OnClick(func(ctx *application.Context) {
		quit()
	})

	tray.SetMenu(menu)

	log.Printf("timo: system tray initialized")
	return tray
}

// UpdateTrayStatus rebuilds the tray menu to reflect current settings.
// Called when settings change.
func UpdateTrayStatus(tray *application.SystemTray, settings TimoSettings) {
	menu := application.NewMenu()

	menu.Add("显示/隐藏 Timo").OnClick(func(ctx *application.Context) {
		if mainWindow != nil {
			if mainWindow.IsMinimised() || !mainWindow.IsVisible() {
				mainWindow.Show()
			} else {
				mainWindow.Minimise()
			}
		}
	})

	menu.Add("设置").OnClick(func(ctx *application.Context) {
		if mainApp != nil {
			mainApp.Event.Emit("open-settings", nil)
		}
	})

	menu.AddSeparator()

	// Theme submenu with checkmarks
	themeMenu := menu.AddSubmenu("主题")

	darkItem := themeMenu.Add("深色")
	if settings.Theme == "dark" {
		darkItem.SetChecked(true)
	}
	darkItem.OnClick(func(ctx *application.Context) {
		s, _ := LoadSettings()
		s.Theme = "dark"
		SaveSettings(s)
		if mainApp != nil {
			mainApp.Event.Emit("settings-loaded", &s)
		}
		UpdateTrayStatus(tray, s)
	})

	lightItem := themeMenu.Add("浅色")
	if settings.Theme == "light" {
		lightItem.SetChecked(true)
	}
	lightItem.OnClick(func(ctx *application.Context) {
		s, _ := LoadSettings()
		s.Theme = "light"
		SaveSettings(s)
		if mainApp != nil {
			mainApp.Event.Emit("settings-loaded", &s)
		}
		UpdateTrayStatus(tray, s)
	})

	frostedItem := themeMenu.Add("磨砂")
	if settings.Theme == "frosted" {
		frostedItem.SetChecked(true)
	}
	frostedItem.OnClick(func(ctx *application.Context) {
		s, _ := LoadSettings()
		s.Theme = "frosted"
		SaveSettings(s)
		if mainApp != nil {
			mainApp.Event.Emit("settings-loaded", &s)
		}
		UpdateTrayStatus(tray, s)
	})

	menu.AddSeparator()

	menu.Add("退出").OnClick(func(ctx *application.Context) {
		if mainApp != nil {
			mainApp.Quit()
		}
	})

	tray.SetMenu(menu)
}
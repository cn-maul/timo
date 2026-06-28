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

	// Build right-click menu
	menu := application.NewMenu()

	menu.Add("显示/隐藏 Timo").OnClick(func(ctx *application.Context) {
		toggleWindow()
	})

	menu.Add("设置").OnClick(func(ctx *application.Context) {
		openSettings()
	})

	menu.AddSeparator()

	// Read-only status items (informational, not clickable)
	statusPriority := menu.Add("优先级: AI > 媒体")
	statusPriority.SetEnabled(false)

	statusTheme := menu.Add("主题: 深色")
	statusTheme.SetEnabled(false)

	menu.AddSeparator()

	menu.Add("退出").OnClick(func(ctx *application.Context) {
		quit()
	})

	tray.SetMenu(menu)

	log.Printf("timo: system tray initialized")
	return tray
}

// UpdateTrayStatus updates the read-only status items in the tray menu to
// reflect current settings. Called when settings change.
func UpdateTrayStatus(tray *application.SystemTray, settings TimoSettings) {
	// Rebuild the entire menu since Wails v3 MenuItems don't have a simple
	// "update label" API that works across all platforms.
	menu := application.NewMenu()

	menu.Add("显示/隐藏 Timo").OnClick(func(ctx *application.Context) {
		// toggle window
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

	// Build priority label
	priorityLabels := map[string]string{
		"ai":    "AI 编码",
		"media": "媒体播放",
	}
	var priorityStr string
	for i, p := range settings.DisplayPriority {
		if i > 0 {
			priorityStr += " > "
		}
		if label, ok := priorityLabels[p]; ok {
			priorityStr += label
		} else {
			priorityStr += p
		}
	}
	si := menu.Add("优先级: " + priorityStr)
	si.SetEnabled(false)

	themeLabels := map[string]string{
		"dark":    "深色",
		"light":   "浅色",
		"frosted": "磨砂",
	}
	themeLabel := "深色"
	if l, ok := themeLabels[settings.Theme]; ok {
		themeLabel = l
	}
	si2 := menu.Add("主题: " + themeLabel)
	si2.SetEnabled(false)

	menu.AddSeparator()

	menu.Add("退出").OnClick(func(ctx *application.Context) {
		if mainApp != nil {
			mainApp.Quit()
		}
	})

	tray.SetMenu(menu)
}

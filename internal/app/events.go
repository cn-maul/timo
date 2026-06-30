package app

import (
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// registerAllEventHandlers wires all application-level event handlers that were
// previously registered inline in main().
func registerAllEventHandlers(
	app *application.App,
	tray *application.SystemTray,
	bus EventBus,
	win WindowController,
	quit func(),
	settingsWinMgr *SettingsWindowManager,
) {
	// Keep tray menu in sync when settings change
	app.Event.On("settings-updated", func(event *application.CustomEvent) {
		if tray != nil {
			if settings, ok := event.Data.(*TimoSettings); ok {
				UpdateTrayStatus(tray, *settings, bus, win, quit)
			}
		}
	})

	// Open settings from tray (after menu rebuild) and other sources
	app.Event.On("open-settings", func(event *application.CustomEvent) {
		if settingsWinMgr != nil {
			settingsWinMgr.Open()
		}
	})

	// Close settings window (frontend "✕" button) — hide, keep it openable again.
	app.Event.On("close-settings", func(event *application.CustomEvent) {
		if settingsWinMgr != nil {
			settingsWinMgr.Close()
		}
	})

	// Hooks status query
	app.Event.On("get-hooks-status", func(event *application.CustomEvent) {
		status := getHooksStatus()
		app.Event.Emit("hooks-status", status)
	})

	// Inject hooks
	app.Event.On("inject-hook", func(event *application.CustomEvent) {
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
		app.Event.Emit("hooks-feedback", msg)
		// Refresh status
		status := getHooksStatus()
		app.Event.Emit("hooks-status", status)
	})

	// Remove hooks
	app.Event.On("remove-hook", func(event *application.CustomEvent) {
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
		app.Event.Emit("hooks-feedback", msg)
		status := getHooksStatus()
		app.Event.Emit("hooks-status", status)
	})
}

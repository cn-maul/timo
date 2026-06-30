package app

import "github.com/wailsapp/wails/v3/pkg/application"

// EventBus 抽象了向前端发射 Wails 事件的能力。
// mainApp.Event (*EventManager) 满足此接口。
type EventBus interface {
	Emit(event string, optionalData ...any) bool
}

// WindowController 抽象了对主 notched 窗口的操作。
// *application.WebviewWindow (通过 application.Window 接口) 满足此接口。
type WindowController interface {
	Show() application.Window
	Minimise() application.Window
	Focus()
	Hide() application.Window
	UnMinimise()
	IsMinimised() bool
	IsVisible() bool
	GetScreen() (*application.Screen, error)
}

// AppLifecycle 抽象了应用退出能力。
// mainApp (*application.App) 满足此接口。
type AppLifecycle interface {
	Quit()
}

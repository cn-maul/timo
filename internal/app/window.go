package app

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	"timo/internal/media"
)

func init() {
	application.RegisterEvent[*media.MediaInfo]("media-update")
	application.RegisterEvent[*Notification]("notification")
	application.RegisterEvent[*SystemStats]("sys-stats")
	application.RegisterEvent[*TimoSettings]("settings-updated")
}

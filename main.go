package main

import (
	"embed"

	"timo/internal/app"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app.Run(assets)
}

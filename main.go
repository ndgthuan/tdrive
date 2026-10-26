package main

import (
	"embed"
	"log"
	"os"
	"runtime"

	"TDrive/internal/app"
)

//go:embed all:frontend/dist
var assets embed.FS

// appVersion is stamped by release builds with -X main.appVersion.
var appVersion = "dev"

func main() {
	if err := configureLinuxWebKitEnvironment(); err != nil {
		log.Fatal(err)
	}
	if err := app.Run(assets, appVersion); err != nil {
		log.Fatal(err)
	}
}

// configureLinuxWebKitEnvironment must run before app.Run initializes WebKit,
// including when a desktop shortcut launches the binary without AppRun.
func configureLinuxWebKitEnvironment() error {
	const name = "WEBKIT_DISABLE_COMPOSITING_MODE"
	current := os.Getenv(name)
	value := webKitCompositingModeDefault(runtime.GOOS, current)
	if value == current {
		return nil
	}
	return os.Setenv(name, value)
}

func webKitCompositingModeDefault(goos, current string) string {
	if goos == "linux" && current == "" {
		return "1"
	}
	return current
}

// Copperline GUI is the shared desktop/Android relay frontend.
package main

import (
	_ "embed"
	"flag"
	"fmt"
	"os"

	"copperline/internal/gui"
	buildversion "copperline/internal/version"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
)

// Icon.png is the single source for the desktop window icon and Android/desktop
// package icon, so runtime and launcher artwork cannot drift apart.
//
//go:embed Icon.png
var iconData []byte

var icon = fyne.NewStaticResource("copperline.png", iconData)

func main() {
	configPath := flag.String("config", "", "optional desktop relay TOML file (otherwise use GUI app storage)")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("Copperline GUI", buildversion.Current)
		return
	}

	a := app.NewWithID("ca.cerberusgames.copperline")
	g, err := gui.New(a, *configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Copperline GUI:", err)
		os.Exit(1)
	}
	g.SetIcon(icon)
	g.Run()
}

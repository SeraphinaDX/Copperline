// Copperline GUI is the shared desktop/Android relay frontend.
package main

import (
	"flag"
	"fmt"
	"os"

	"copperline/internal/gui"
	buildversion "copperline/internal/version"
	"fyne.io/fyne/v2/app"
)

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
	g.Run()
}

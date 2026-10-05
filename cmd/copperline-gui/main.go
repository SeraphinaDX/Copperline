// Copperline GUI is the shared desktop/Android relay frontend.
package main

import (
	"flag"
	"fmt"
	"os"

	"copperline/internal/gui"
	"fyne.io/fyne/v2/app"
)

func main() {
	configPath := flag.String("config", "", "optional desktop relay TOML file (otherwise use GUI app storage)")
	flag.Parse()
	a := app.NewWithID("ca.cerberusgames.copperline")
	g, err := gui.New(a, *configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Copperline GUI:", err)
		os.Exit(1)
	}
	g.Run()
}

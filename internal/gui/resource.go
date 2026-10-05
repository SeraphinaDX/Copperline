package gui

import (
	_ "embed"
	"fyne.io/fyne/v2"
)

//go:embed icon.svg
var iconData []byte

var Icon = fyne.NewStaticResource("copperline.svg", iconData)

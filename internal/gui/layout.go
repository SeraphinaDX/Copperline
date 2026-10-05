package gui

import (
	"fyne.io/fyne/v2"
)

// The same chat surface fits a phone. Desktop sidebars are placed alongside it
// when width permits; narrow windows use the Channels/Users dialog buttons.
type responsiveLayout struct {
	chat            fyne.CanvasObject
	channels, users fyne.CanvasObject
}

func (l *responsiveLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(320, 400)
}
func (l *responsiveLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	// The first object is the toolbar; the border container's center is chat.
	toolbar := objects[0]
	toolbar.Move(fyne.NewPos(0, 0))
	toolbar.Resize(fyne.NewSize(size.Width, toolbar.MinSize().Height))
	y := toolbar.MinSize().Height + 4
	height := size.Height - y
	if size.Width >= 800 {
		l.channels.Show()
		l.users.Show()
		l.channels.Move(fyne.NewPos(0, y))
		l.channels.Resize(fyne.NewSize(220, height))
		l.users.Move(fyne.NewPos(size.Width-160, y))
		l.users.Resize(fyne.NewSize(160, height))
		l.chat.Move(fyne.NewPos(224, y))
		l.chat.Resize(fyne.NewSize(size.Width-388, height))
	} else {
		l.channels.Hide()
		l.users.Hide()
		l.chat.Move(fyne.NewPos(0, y))
		l.chat.Resize(fyne.NewSize(size.Width, height))
	}
}

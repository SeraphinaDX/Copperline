package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

// secondaryLabel behaves like a normal label for left-click list selection,
// while also letting desktop nick-list rows expose a right-click context menu.
type secondaryLabel struct {
	widget.Label
	onSecondary func(*fyne.PointEvent)
}

func newSecondaryLabel() *secondaryLabel {
	label := &secondaryLabel{}
	label.ExtendBaseWidget(label)
	return label
}

func (l *secondaryLabel) TappedSecondary(ev *fyne.PointEvent) {
	if l.onSecondary != nil {
		l.onSecondary(ev)
	}
}

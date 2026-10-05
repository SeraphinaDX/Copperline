package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

// secondaryLabel behaves like a normal label for ordinary list selection,
// while exposing Fyne's secondary action: right-click on desktop or long-press on mobile.
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

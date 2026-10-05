package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

// composerEntry is the desktop message entry. A normal single-line Fyne Entry
// lets Tab move focus, but IRC users expect Tab to complete nicknames.
type composerEntry struct {
	widget.Entry
	captureTab bool
	onTab      func() bool
	onNonTab   func()
}

func newComposerEntry(captureTab bool, onTab func() bool, onNonTab func()) *composerEntry {
	e := &composerEntry{captureTab: captureTab, onTab: onTab, onNonTab: onNonTab}
	e.Wrapping = fyne.TextWrap(fyne.TextTruncateClip)
	e.ExtendBaseWidget(e)
	return e
}

// AcceptsTab asks Fyne to deliver Tab to TypedKey on desktop instead of moving
// focus to the next widget. Mobile keeps the normal focus behaviour.
func (e *composerEntry) AcceptsTab() bool {
	return e.captureTab
}

func (e *composerEntry) TypedKey(key *fyne.KeyEvent) {
	if e.captureTab && key.Name == fyne.KeyTab {
		if e.onTab != nil {
			e.onTab()
		}
		return
	}
	if e.onNonTab != nil {
		e.onNonTab()
	}
	e.Entry.TypedKey(key)
}

package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/mobile"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// composerEntry captures desktop nickname completion and keeps Android sending
// inside the entry's touch/focus area, so Send cannot dismiss the keyboard.
type composerEntry struct {
	widget.Entry
	captureTab     bool
	onTab          func() bool
	onNonTab       func()
	onFocus        func(bool)
	onSend         func()
	focused        bool
	sendTouch      bool
	sendEnabled    bool
	sendLabel      *canvas.Text
	sendBackground *canvas.Rectangle
}

// The action is deliberately drawn with non-interactive canvas objects. A
// separate Button makes Fyne's mobile driver unfocus the entry on touch-down,
// hiding the keyboard and moving the button before touch-up can activate it.
func (e *composerEntry) addMobileSend(send func()) {
	e.onSend = send
	e.sendLabel = canvas.NewText("Send", theme.DisabledColor())
	e.sendLabel.Alignment = fyne.TextAlignCenter
	e.sendLabel.TextStyle.Bold = true
	e.sendBackground = canvas.NewRectangle(theme.ButtonColor())
	e.sendBackground.SetMinSize(fyne.NewSize(64, 44))
	e.ActionItem = container.NewStack(e.sendBackground, e.sendLabel)
}

func (e *composerEntry) setSendEnabled(enabled bool) {
	e.sendEnabled = enabled
	e.Refresh()
}

func (e *composerEntry) Refresh() {
	if e.sendLabel != nil {
		th, variant := e.Theme(), fyne.CurrentApp().Settings().ThemeVariant()
		name := theme.ColorNameDisabled
		if e.sendEnabled && !e.Disabled() {
			name = theme.ColorNamePrimary
		}
		e.sendLabel.Color = th.Color(name, variant)
		e.sendLabel.TextSize = th.Size(theme.SizeNameText)
		e.sendBackground.FillColor = th.Color(theme.ColorNameButton, variant)
		e.sendBackground.CornerRadius = th.Size(theme.SizeNameButtonRadius)
	}
	e.Entry.Refresh()
}

func (e *composerEntry) MinSize() fyne.Size {
	size := e.Entry.MinSize()
	if e.onSend != nil {
		size.Height = max(size.Height, 52)
	}
	return size
}

func (e *composerEntry) Keyboard() mobile.KeyboardType {
	if e.onSend != nil {
		// Android's single-line "Done" action dismisses the IME itself. Use
		// Return while keeping Entry single-line: Return still calls OnSubmitted.
		return mobile.DefaultKeyboard
	}
	return e.Entry.Keyboard()
}

func (e *composerEntry) FocusGained() {
	e.focused = true
	e.Entry.FocusGained()
	if e.onFocus != nil {
		e.onFocus(true)
	}
}

func (e *composerEntry) FocusLost() {
	e.focused = false
	e.Entry.FocusLost()
	if e.onFocus != nil {
		e.onFocus(false)
	}
}

func (e *composerEntry) inSend(pos fyne.Position) bool {
	return e.onSend != nil && pos.X >= e.ActionItem.Position().X &&
		pos.X < e.ActionItem.Position().X+e.ActionItem.Size().Width &&
		pos.Y >= 0 && pos.Y < e.Size().Height
}

func (e *composerEntry) TouchDown(ev *mobile.TouchEvent) {
	e.sendTouch = e.inSend(ev.Position)
	if e.sendTouch {
		return
	}
	e.Entry.TouchDown(ev)
}

func (e *composerEntry) TouchCancel(ev *mobile.TouchEvent) {
	e.sendTouch = false
	e.Entry.TouchCancel(ev)
}

func (e *composerEntry) TouchUp(ev *mobile.TouchEvent) {
	send := e.sendTouch && e.inSend(ev.Position)
	e.sendTouch = false
	if send && e.sendEnabled && !e.Disabled() {
		// Activate on release rather than waiting for Entry's double-tap
		// recognition. The later Tapped/DoubleTapped events ignore this area.
		e.onSend()
	}
	e.Entry.TouchUp(ev)
}

func (e *composerEntry) Dragged(ev *fyne.DragEvent) {
	if e.sendTouch {
		e.sendTouch = false
		return
	}
	e.Entry.Dragged(ev)
}

func (e *composerEntry) Tapped(ev *fyne.PointEvent) {
	if e.inSend(ev.Position) {
		return
	}
	e.Entry.Tapped(ev)
}

func (e *composerEntry) DoubleTapped(ev *fyne.PointEvent) {
	if !e.inSend(ev.Position) {
		e.Entry.DoubleTapped(ev)
	}
}

func (e *composerEntry) TappedSecondary(ev *fyne.PointEvent) {
	if !e.inSend(ev.Position) {
		e.Entry.TappedSecondary(ev)
	}
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

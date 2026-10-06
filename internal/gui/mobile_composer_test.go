package gui

import (
	"fmt"
	"math"
	"testing"
	"time"

	"copperline/internal/model"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/mobile"
	"fyne.io/fyne/v2/test"
)

func TestMobileEnterAndSendKeepComposerFocused(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	w := a.NewWindow("composer")
	e := newComposerEntry(false, nil, nil)
	var sent []string
	submit := func() {
		sent = append(sent, e.Text)
		e.SetText("")
	}
	e.addMobileSend(submit)
	e.OnSubmitted = func(string) { submit() }
	e.setSendEnabled(true)
	w.SetContent(e)
	w.Resize(fyne.NewSize(360, 100))
	w.Canvas().Focus(e)

	if e.Keyboard() != mobile.DefaultKeyboard || e.MultiLine {
		t.Fatal("mobile composer must use Return without making messages multiline")
	}
	e.SetText("from keyboard")
	e.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	pos := e.ActionItem.Position().Add(fyne.NewPos(10, 10))
	ev := &mobile.TouchEvent{PointEvent: fyne.PointEvent{Position: pos}}
	for _, text := range []string{"first tap", "second tap"} {
		e.SetText(text)
		e.TouchDown(ev)
		if w.Canvas().Focused() != e {
			t.Fatal("Send touch-down changed focus before activation")
		}
		e.TouchUp(ev)
		// Fyne also delivers delayed tap/double-tap events after touch-up.
		e.Tapped(&fyne.PointEvent{Position: pos})
		e.DoubleTapped(&fyne.PointEvent{Position: pos})
		if w.Canvas().Focused() != e || !e.focused || e.Text != "" {
			t.Fatal("sending lost focus or left submitted text in the composer")
		}
	}
	if fmt.Sprint(sent) != "[from keyboard first tap second tap]" {
		t.Fatalf("unexpected/duplicate sends: %q", sent)
	}

	// Any interactive action child would become the driver's touch target,
	// triggering its automatic unfocus and keyboard dismissal again.
	var checkPassive func(fyne.CanvasObject)
	checkPassive = func(o fyne.CanvasObject) {
		if _, ok := o.(fyne.Focusable); ok {
			t.Fatal("Send action steals composer focus")
		}
		if _, ok := o.(mobile.Touchable); ok {
			t.Fatal("Send action intercepts composer touches")
		}
		if c, ok := o.(*fyne.Container); ok {
			for _, child := range c.Objects {
				checkPassive(child)
			}
		}
	}
	checkPassive(e.ActionItem)
}

func TestMobileSendCancellationAndBusyState(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	e := newComposerEntry(false, nil, nil)
	sends := 0
	e.addMobileSend(func() { sends++ })
	e.Resize(fyne.NewSize(360, 52))
	pos := e.ActionItem.Position().Add(fyne.NewPos(10, 10))
	ev := &mobile.TouchEvent{PointEvent: fyne.PointEvent{Position: pos}}
	e.setSendEnabled(false)
	e.TouchDown(ev)
	e.TouchUp(ev)
	e.setSendEnabled(true)
	e.TouchDown(ev)
	e.TouchCancel(ev)
	e.TouchUp(ev)
	e.TouchDown(ev)
	e.TouchUp(&mobile.TouchEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(10, 10)}})
	e.TouchDown(ev)
	e.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: pos}})
	e.TouchUp(ev)
	e.Disable()
	e.TouchDown(ev)
	e.TouchUp(ev)
	if sends != 0 {
		t.Fatalf("disabled or cancelled touch sent %d messages", sends)
	}
}

func TestMobileKeyboardViewportShowsLatestMessages(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	g := &App{app: a, window: a.NewWindow("mobile chat"), mobile: true,
		state: model.New(1000), drafts: map[string]string{}, clearThrough: map[string]uint64{}, done: make(chan struct{})}
	g.build()
	defer g.shutdown()
	visibleSend := false
	for _, o := range test.WidgetRenderer(g.entry).Objects() {
		if o == g.entry.ActionItem {
			visibleSend = true
		}
	}
	if !visibleSend {
		t.Fatal("Send action is missing from the mobile entry renderer")
	}
	g.state.Select("test", "#chat")
	for i := range 60 {
		g.state.Add(model.Message{Time: time.Now(), Server: "test", Target: "#chat", Nick: "alice",
			Text: fmt.Sprintf("Message %d has enough words to wrap across the phone viewport.", i), Kind: model.KindMessage})
	}
	g.refresh()
	g.entry.Enable()
	g.setSendEnabled(true)
	g.window.Resize(fyne.NewSize(360, 720))
	g.scroll.ScrollToTop()
	g.window.Canvas().Focus(g.entry)
	for _, size := range []fyne.Size{fyne.NewSize(360, 320), fyne.NewSize(360, 280), fyne.NewSize(640, 240), fyne.NewSize(360, 720)} {
		g.window.Resize(size)
		if g.root.Objects[0].Visible() {
			t.Fatal("mobile navigation consumes conversation space while typing")
		}
		bottom := g.scroll.Offset.Y + g.scroll.Size().Height
		if math.Abs(float64(bottom-g.transcript.MinSize().Height)) > 1 {
			t.Fatalf("latest messages hidden after resize to %v: visible bottom=%f transcript=%f", size, bottom, g.transcript.MinSize().Height)
		}
		pos := a.Driver().AbsolutePositionForObject(g.entry)
		if pos.Y+g.entry.Size().Height > g.window.Canvas().Size().Height+1 || g.scroll.Size().Height < 32 {
			t.Fatalf("composer or conversation does not fit keyboard viewport %v", size)
		}
	}
	g.scroll.ScrollToTop()
	g.root.Refresh()
	if g.scroll.Offset.Y != 0 {
		t.Fatal("ordinary refresh must not interrupt manual scrollback")
	}
	g.window.Canvas().Unfocus()
	if !g.root.Objects[0].Visible() {
		t.Fatal("navigation was not restored after leaving the composer")
	}
}

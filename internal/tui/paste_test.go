package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/gdamore/tcell/v3"
	ui "github.com/metaspartan/gotui/v5"
)

func TestBracketedPasteIsOneEvent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	source := make(chan tcell.Event, 10)
	events := pasteEvents(ctx, source)
	text := strings.Repeat("long line\n", 5000)
	go func() {
		source <- tcell.NewEventPaste(true)
		for _, r := range text {
			source <- tcell.NewEventKey(tcell.KeyRune, string(r), tcell.ModNone)
		}
		source <- tcell.NewEventPaste(false)
		close(source)
	}()
	select {
	case e := <-events:
		k, ok := e.(*tcell.EventKey)
		if !ok || k.Key() != pasteKey || k.Str() != text {
			t.Fatalf("paste changed: event=%T key=%v len=%d want=%d prefix=%q", e, k.Key(), len(k.Str()), len(text), k.Str()[:min(30, len(k.Str()))])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("paste aggregation stalled")
	}
	if _, ok := <-events; ok {
		t.Fatal("paste generated additional typing or Enter events")
	}
}

func TestPasteChunksPreserveUnicodeAndCommandsAsText(t *testing.T) {
	text := "/quit\n" + strings.Repeat("é🌸", 150) + "\nlast line\n"
	lines := pasteMessages(text)
	if lines[0] != "/quit" || lines[len(lines)-1] != "last line" {
		t.Fatal("lines changed")
	}
	for _, line := range lines {
		if len(line) > 240 || !utf8.ValidString(line) {
			t.Fatal("invalid chunk")
		}
	}
	if strings.Join(lines[1:len(lines)-1], "") != strings.Repeat("é🌸", 150) {
		t.Fatal("Unicode text lost")
	}
}

type pasteBackend struct {
	failingSendBackend
	calls   chan string
	release chan struct{}
	fail    bool
}

func (b *pasteBackend) SendMessage(_, _, text string) error {
	b.calls <- text
	if b.release != nil {
		<-b.release
	}
	if b.fail {
		return errors.New("unconfirmed")
	}
	return nil
}

func TestPasteSendKeepsUIResponsiveAndRetainsFailure(t *testing.T) {
	a := newCatchupApp(t)
	server := a.irc.ServerNames()[0]
	a.state.Select(server, "#chat")
	backend := &pasteBackend{calls: make(chan string, 8), release: make(chan struct{}), fail: true}
	a.irc = backend
	a.input.Text = "prefix "
	a.input.Cursor = len([]rune(a.input.Text))
	a.handleUIEvent(ui.Event{Type: ui.KeyboardEvent, Payload: tcell.NewEventKey(pasteKey, "first\n/quit\nthird", 0)})
	if a.input.Text != "prefix first\n/quit\nthird" {
		t.Fatalf("draft=%q", a.input.Text)
	}
	select {
	case <-backend.calls:
		t.Fatal("paste sent without Enter")
	default:
	}
	a.handleKey(ui.Event{Type: ui.KeyboardEvent, ID: "<Enter>"})
	select {
	case text := <-backend.calls:
		if text != "prefix first" {
			t.Fatal(text)
		}
	case <-time.After(time.Second):
		t.Fatal("no send")
	}
	a.handleKey(ui.Event{Type: ui.KeyboardEvent, ID: "x"})
	if a.input.Text != "x" {
		t.Fatal("typing blocked during send")
	}
	close(backend.release)
	select {
	case r := <-a.pasteResults:
		a.finishPasteSend(r)
	case <-time.After(time.Second):
		t.Fatal("no failure result")
	}
	if a.pasteSend == nil || a.pasteSend.active || len(a.pasteSend.remaining) != 3 {
		t.Fatal("unconfirmed text was lost")
	}
	if a.input.Text != "x" {
		t.Fatal("failure overwrote new draft")
	}
	select {
	case <-backend.calls:
		t.Fatal("sent more lines after failure")
	default:
	}
	if a.stopped.Load() {
		t.Fatal("pasted /quit executed")
	}
}

func TestPasteCancellationDoesNotSendRemainingLines(t *testing.T) {
	a := newCatchupApp(t)
	server := a.irc.ServerNames()[0]
	a.state.Select(server, "#chat")
	backend := &pasteBackend{calls: make(chan string, 8), release: make(chan struct{})}
	a.irc = backend
	a.insertPaste("one\ntwo\nthree")
	a.startPasteSend()
	select {
	case <-backend.calls:
	case <-time.After(time.Second):
		t.Fatal("no first send")
	}
	a.pasteSend.cancel()
	close(backend.release)
	deadline := time.After(time.Second)
	for a.pasteSend.active {
		select {
		case r := <-a.pasteResults:
			a.finishPasteSend(r)
		case <-deadline:
			t.Fatal("cancel did not finish")
		}
	}
	if a.pasteSend.confirmed != 1 || strings.Join(a.pasteSend.remaining, "\n") != "two\nthree" {
		t.Fatal("wrong cancellation boundary")
	}
	select {
	case <-backend.calls:
		t.Fatal("sent after cancellation")
	default:
	}
}

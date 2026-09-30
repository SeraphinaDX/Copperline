package tui

import (
	"errors"
	"testing"

	"copperline/internal/model"
)

type closeBackend struct {
	*slashBackend
	parts           int
	channel, reason string
	onPart          func()
}

func (b *closeBackend) Part(server, channel, reason string) error {
	b.parts++
	b.server, b.channel, b.reason = server, channel, reason
	if b.onPart != nil {
		b.onPart()
	}
	return b.err
}

func (b *closeBackend) Join(string, string, string) error { return nil }

func TestCloseChannelPartsAndStaysClosed(t *testing.T) {
	a, base := newSlashApp()
	b := &closeBackend{slashBackend: base}
	a.irc = b
	b.onPart = func() {
		a.onMessage(model.Message{Server: "test", Target: "#CHAT", Kind: model.KindSystem, Text: "tester left"})
	}
	a.execute("/close goodbye")
	if b.parts != 1 || b.server != "test" || b.channel != "#chat" || b.reason != "goodbye" {
		t.Fatalf("incorrect PART request: %+v", b)
	}
	for _, replay := range []bool{false, true} {
		a.onMessage(model.Message{Server: "test", Target: "#CHAT", Nick: "alice", Kind: model.KindMessage, Text: "queued", Replay: replay})
	}
	a.onBackendUpdate()
	if a.state.Find("test", "#chat") != nil || !a.isBufferClosed("test", "#chat") {
		t.Fatal("traffic or snapshot reopened closed channel")
	}
	a.execute("/join #chat")
	a.onMessage(model.Message{Server: "test", Target: "#chat", Nick: "alice", Kind: model.KindMessage, Text: "after rejoin"})
	channel := a.state.Find("test", "#chat")
	if a.isBufferClosed("test", "#chat") || channel == nil || channel.Messages[len(channel.Messages)-1].Text != "after rejoin" {
		t.Fatal("explicit join did not restore channel delivery")
	}
}

func TestCloseFailedPartKeepsBufferAndDraft(t *testing.T) {
	a, base := newSlashApp()
	b := &closeBackend{slashBackend: base}
	b.err = errors.New("relay unavailable")
	a.irc = b
	a.input.Text = "unfinished"
	a.execute("/close")
	channel := a.state.Find("test", "#chat")
	if channel == nil || a.isBufferClosed("test", "#chat") || a.input.Text != "unfinished" {
		t.Fatal("failed PART discarded buffer or draft")
	}
	if len(channel.Messages) == 0 || channel.Messages[len(channel.Messages)-1].Kind != model.KindError {
		t.Fatal("failed PART did not display an error")
	}
}

func TestCloseQueryDoesNotPart(t *testing.T) {
	a, base := newSlashApp()
	b := &closeBackend{slashBackend: base}
	a.irc = b
	a.selectBuffer("test", "alice")
	a.execute("/close")
	if b.parts != 0 || a.state.Find("test", "alice") != nil {
		t.Fatal("query close sent PART or kept the buffer")
	}
}

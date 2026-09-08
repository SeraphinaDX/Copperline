package tui

import (
	"strings"
	"testing"
	"time"

	"copperline/internal/model"
)

func TestChannelSwitchAcknowledgesActivity(t *testing.T) {
	for _, selection := range []string{"number", "relative", "target", "key", "unread"} {
		t.Run(selection, func(t *testing.T) {
			a := newCatchupApp(t)
			server := a.irc.ServerNames()[0]
			a.state = model.New(100)
			a.state.Select(server, "*server*")
			a.rebuildCurrent()
			addCatchupMessages(a, server, "#two", "first", "second")
			switch selection {
			case "number":
				a.selectBufferNumber(2)
			case "relative":
				a.selectRelative(1)
			case "target":
				a.state.Select(server, "#two")
			case "key":
				a.state.SelectKey(model.Key(server, "#two"))
			case "unread":
				a.selectNextUnread()
			}
			a.rebuildCurrent()
			a.rebuildSidebar()
			if b := a.state.CurrentInfo(); b.Target != "#two" || b.Unread != 0 {
				t.Fatalf("activity not cleared: %#v", b)
			}
			if !a.follow || a.transcript.SelectedRow != 1 {
				t.Fatal("channel did not resume live follow")
			}
			if strings.Contains(a.sidebar.Rows[1], "(2)") {
				t.Fatal("stale activity badge")
			}
			addCatchupMessages(a, server, "#two", "live")
			a.rebuildCurrent()
			if a.state.CurrentInfo().Unread != 0 {
				t.Fatal("live-follow message kept activity badge")
			}
		})
	}
}

func TestFormattedMessagesAndBacklogDoNotExposeColorDigits(t *testing.T) {
	a := newCatchupApp(t)
	for _, replay := range []bool{false, true} {
		for _, kind := range []model.Kind{model.KindMessage, model.KindAction, model.KindNotice} {
			msg := model.Message{Time: time.Unix(0, 0), Nick: "alice", Text: "\x0302hello\x0f 02 literal", Kind: kind, Replay: replay}
			row := a.theme.formatMessage(msg, "15:04")
			var plain strings.Builder
			for _, cell := range parseTranscriptStyles(row, a.transcript.TextStyle) {
				plain.WriteRune(cell.Rune)
			}
			if !strings.HasSuffix(plain.String(), "hello 02 literal") {
				t.Fatalf("rendered %q", plain.String())
			}
			if msg.Text != "\x0302hello\x0f 02 literal" {
				t.Fatal("changed original message")
			}
		}
	}
	var backlog strings.Builder
	for _, cell := range parseTranscriptStyles(a.theme.formatLogBacklog("<alice> \x0302hello"), a.transcript.TextStyle) {
		backlog.WriteRune(cell.Rune)
	}
	if backlog.String() != "<alice> hello" {
		t.Fatalf("backlog=%q", backlog.String())
	}
}

func TestSearchIgnoresHiddenColorParameters(t *testing.T) {
	a := newCatchupApp(t)
	server := a.irc.ServerNames()[0]
	a.state.Select(server, "#test")
	addCatchupMessages(a, server, "#test", "\x0302colored text", "02 literal")
	a.rebuildCurrent()
	a.startSearch("02")
	if a.search.label != "search 1/1" || a.transcript.SelectedRow != 1 {
		t.Fatalf("matched hidden formatting: %s, row %d", a.search.label, a.transcript.SelectedRow)
	}
}

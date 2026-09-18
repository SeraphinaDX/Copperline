package tui

import (
	"strings"
	"testing"

	"copperline/internal/model"
	ui "github.com/metaspartan/gotui/v5"
)

func TestBufferDraftsAndCommands(t *testing.T) {
	a := newCatchupApp(t)
	server := a.irc.ServerNames()[0]
	a.selectBuffer(server, "#one")
	a.input.Text, a.input.Cursor, a.pasteLiteral = "héllo\n/quit", 2, true
	a.selectBuffer(server, "#two")
	if a.input.Text != "" || a.pasteLiteral {
		t.Fatal("draft leaked to another channel")
	}
	a.input.Text, a.input.Cursor = "second draft", 4
	a.selectBufferKey(model.Key(server, "#ONE"))
	if a.input.Text != "héllo\n/quit" || a.input.Cursor != 2 || !a.pasteLiteral {
		t.Fatal("multiline draft or cursor was not restored")
	}
	a.pasteLiteral = false
	// A slash command clears only its source input, never the destination draft.
	a.input.Text = "/query alice"
	a.handleKey(ui.Event{ID: "<Enter>"})
	if a.input.Text != "" || a.state.CurrentInfo().Target != "alice" {
		t.Fatal("query command left stale input")
	}
	a.selectBuffer(server, "#two")
	if a.input.Text != "second draft" || a.input.Cursor != 4 {
		t.Fatal("destination draft lost")
	}
	a.execute("/close")
	a.selectBuffer(server, "#two")
	if a.input.Text != "" {
		t.Fatal("closed buffer retained its draft")
	}
}

func TestPasteRestoreProtectsDestinationDraft(t *testing.T) {
	a := newCatchupApp(t)
	server := a.irc.ServerNames()[0]
	a.selectBuffer(server, "#paste")
	a.input.Text = "keep this"
	a.selectBuffer(server, "#elsewhere")
	a.pasteSend = &pasteSend{server: server, target: "#paste", remaining: []string{"one", "/quit"}}
	a.executePaste("restore")
	if a.input.Text != "keep this" || a.pasteSend == nil {
		t.Fatal("restore overwrote destination draft")
	}
	a.input.Text = ""
	a.executePaste("restore")
	if a.input.Text != "one\n/quit" || !a.pasteLiteral || a.pasteSend != nil {
		t.Fatal("paste not restored as literal text")
	}
}

func TestHistoryBrowsingSurvivesEditingAnotherBuffer(t *testing.T) {
	a := newCatchupApp(t)
	server := a.irc.ServerNames()[0]
	a.selectBuffer(server, "#one")
	a.addInputHistory("previous message")
	a.input.Text = "unfinished reply"
	a.inputHistoryPrevious()
	a.selectBuffer(server, "#two")
	a.handleKey(ui.Event{ID: "x"})
	a.selectBuffer(server, "#one")
	a.inputHistoryNext()
	if a.input.Text != "unfinished reply" {
		t.Fatal("editing another buffer discarded the history draft")
	}
}

func TestClearKeepsHistoryHiddenAcrossRevisitAndReplay(t *testing.T) {
	a := newCatchupApp(t)
	server := a.irc.ServerNames()[0]
	a.selectBuffer(server, "#one")
	addCatchupMessages(a, server, "#one", "old message")
	a.rebuildCurrent()
	old := a.state.Current().Messages[0]
	a.input.Text = "unfinished"
	a.execute("/clear")
	a.rebuildCurrent()
	if len(a.transcript.Rows) != 1 || a.transcript.Rows[0] != "No messages yet." || a.input.Text != "unfinished" {
		t.Fatal("clear did not clear only the view")
	}
	a.selectBuffer(server, "#two")
	a.rebuildCurrent()
	old.Replay = true
	a.onMessage(old)
	addCatchupMessages(a, server, "#one", "new message")
	a.selectBuffer(server, "#one")
	a.rebuildCurrent()
	if len(a.transcript.Rows) != 1 || !strings.Contains(a.transcript.Rows[0], "new message") {
		t.Fatalf("cleared history reappeared: %v", a.transcript.Rows)
	}
	a.startSearch("old message")
	if a.search.found {
		t.Fatal("search matched cleared rows")
	}
	if len(a.state.Current().Messages) != 2 {
		t.Fatal("clear destroyed history or replay duplicated it")
	}
}

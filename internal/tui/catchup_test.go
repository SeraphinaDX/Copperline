package tui

import (
	"strings"
	"testing"
	"time"

	"copperline/internal/config"
	"copperline/internal/model"
)

func newCatchupApp(t *testing.T) *App {
	t.Helper()
	cfg, err := config.Load("../../config.example.toml")
	if err != nil {
		t.Fatal(err)
	}
	no := false
	cfg.General.Logging = &no
	cfg.General.LogDir = t.TempDir()
	cfg.Scripting.Enabled = &no
	cfg.Gotify.Enabled = false
	a := New(cfg)
	a.startupComplete.Store(true)
	return a
}

func addCatchupMessages(a *App, server, target string, texts ...string) {
	for _, text := range texts {
		a.state.Add(model.Message{Server: server, Target: target, Nick: "alice", Text: text, Time: time.Unix(123, 0), Kind: model.KindMessage})
	}
}

func TestUnreadSelectionAndEnd(t *testing.T) {
	a := newCatchupApp(t)
	server := a.irc.ServerNames()[0]
	a.state.Select(server, "#one")
	a.rebuildCurrent()
	addCatchupMessages(a, server, "#two", "first", "second", "third")
	a.selectNextUnread()
	a.rebuildCurrent()
	b := a.state.CurrentInfo()
	if b.Target != "#two" || b.Unread != 3 || a.follow {
		t.Fatalf("selection cleared unread: %#v follow=%v", b, a.follow)
	}
	if a.transcript.SelectedRow != 0 || !a.transcript.unread {
		t.Fatal("did not open at first unread")
	}
	a.follow = true
	a.rebuildCurrent()
	if a.state.CurrentInfo().Unread != 0 || a.transcript.unread {
		t.Fatal("following bottom did not mark displayed messages read")
	}
	a.follow = false
	addCatchupMessages(a, server, "#two", "while scrolled")
	a.rebuildCurrent()
	if a.state.CurrentInfo().Unread != 1 {
		t.Fatal("incoming message while scrolled was marked read")
	}
}

func TestSearchNavigationAndBufferIsolation(t *testing.T) {
	a := newCatchupApp(t)
	server := a.irc.ServerNames()[0]
	a.state.Select(server, "#search")
	addCatchupMessages(a, server, "#search", "hello [world]", "unrelated", "HELLO again")
	a.rebuildCurrent()
	a.startSearch("hello")
	a.rebuildCurrent()
	if a.transcript.SelectedRow != 0 || !strings.Contains(a.transcript.Title, "1/2") {
		t.Fatalf("first match: row=%d title=%s", a.transcript.SelectedRow, a.transcript.Title)
	}
	a.moveSearch(1)
	if a.transcript.SelectedRow != 2 {
		t.Fatal("next did not find second match")
	}
	a.moveSearch(1)
	if a.transcript.SelectedRow != 0 {
		t.Fatal("next did not wrap")
	}
	a.moveSearch(-1)
	if a.transcript.SelectedRow != 2 {
		t.Fatal("previous did not wrap")
	}
	a.startSearch("missing")
	a.rebuildCurrent()
	if !strings.Contains(a.transcript.Title, "no matches") {
		t.Fatal("missing no-results feedback")
	}
	a.startSearch("")
	a.rebuildCurrent()
	if a.transcript.searchQuery != "" {
		t.Fatal("empty search did not clear highlighting")
	}
	a.startSearch("hello")
	a.state.Select(server, "#other")
	a.rebuildCurrent()
	if a.search.query != "" {
		t.Fatal("search leaked across buffers")
	}
}

func TestSearchHighlightPreservesLiteralUnicode(t *testing.T) {
	a := newCatchupApp(t)
	tx := a.transcript
	tx.Rows = []string{"Ålice [hello] says HELLO"}
	tx.searchQuery = "hello"
	lines := tx.wrapLogicalRow(0, 100)
	var text strings.Builder
	highlighted := 0
	for _, line := range lines {
		for _, cell := range line {
			text.WriteRune(cell.Rune)
			if cell.Style.Bg != tx.TextStyle.Bg {
				highlighted++
			}
		}
	}
	if text.String() != tx.Rows[0] || highlighted != 10 {
		t.Fatalf("highlight changed text or ranges: %q, %d", text.String(), highlighted)
	}
	if got := searchRanges([]rune("İ [İ]"), "i"); len(got) != 2 || got[1][0] != 3 {
		t.Fatalf("Unicode offsets=%v", got)
	}
}

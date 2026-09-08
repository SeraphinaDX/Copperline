package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	logstore "copperline/internal/logging"
	"copperline/internal/model"
	"github.com/gdamore/tcell/v3"
	ui "github.com/metaspartan/gotui/v5"
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

func TestChannelActivityVisibleOnFirstFrameWithoutTyping(t *testing.T) {
	for _, logging := range []bool{false, true} {
		t.Run(fmt.Sprintf("logging=%v", logging), func(t *testing.T) {
			screen := tcell.NewSimulationScreen("UTF-8")
			if err := screen.Init(); err != nil {
				t.Fatal(err)
			}
			screen.SetSize(100, 24)
			previous := ui.DefaultBackend
			ui.DefaultBackend = &ui.Backend{Screen: screen}
			t.Cleanup(func() { ui.DefaultBackend = previous; screen.Fini() })
			a := newCatchupApp(t)
			a.cfg.General.Logging = &logging
			server := a.irc.ServerNames()[0]
			// Seed genuine pre-session log context to exercise both cache paths.
			if logging {
				dir := filepath.Join(a.cfg.General.LogDir, server)
				if err := os.MkdirAll(dir, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "#two.log"), []byte("12:00 <old> persisted context\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			a.logger = logstore.New(logging, a.cfg.General.LogDir, a.cfg.General.Timestamp)
			a.state = model.New(100)
			a.state.Select(server, "*server*")
			a.render()
			for i := 0; i < 30; i++ {
				addCatchupMessages(a, server, "#two", fmt.Sprintf("message %02d", i))
			}
			assertVisible := func(want string) {
				cells, width, _ := screen.GetContents()
				var visible strings.Builder
				for i, cell := range cells {
					visible.WriteString(string(cell.Runes))
					if (i+1)%width == 0 {
						visible.WriteByte('\n')
					}
				}
				if !strings.Contains(visible.String(), want) {
					t.Fatalf("%q missing on first frame:\n%s", want, visible.String())
				}
			}
			a.selectBufferNumber(2)
			a.render() // Exactly one frame after switching; no typed key or send.
			assertVisible("message 29")
			a.selectBufferNumber(1)
			a.render()
			addCatchupMessages(a, server, "#two", "activity while away")
			a.selectBufferNumber(2)
			a.render()
			assertVisible("activity while away")
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

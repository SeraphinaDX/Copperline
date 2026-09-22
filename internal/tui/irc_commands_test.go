package tui

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"copperline/internal/config"
	ircclient "copperline/internal/irc"
	"copperline/internal/model"
	"github.com/lrstanley/girc"
)

func TestIRCCommand(t *testing.T) {
	tests := []struct{ cmd, args, target, wire string }{
		{"oper", "admin secret", "#chat", "OPER admin secret"},
		{"oper", "admin :secret with spaces", "#chat", "OPER admin ::secret with spaces"},
		{"away", "out for lunch", "#chat", "AWAY :out for lunch"},
		{"away", "#meeting", "#chat", "AWAY #meeting"},
		{"away", "", "#chat", "AWAY"},
		{"back", "", "#chat", "AWAY"},
		{"mode", "+m", "#chat", "MODE #chat +m"},
		{"mode", "-m", "#chat", "MODE #chat -m"},
		{"mode", "#chat +o alice", "#chat", "MODE #chat +o alice"},
		{"mode", "#other", "#chat", "MODE #other"},
		{"mode", "", "#chat", "MODE #chat"},
		{"mode", "+i", "*server*", "MODE tester +i"},
		{"mode", "", "*server*", "MODE tester"},
		{"mode", "tester +i", "#chat", "MODE tester +i"},
		{"mode", "+i", "alice", "MODE tester +i"},
		{"mode", "#other\t+o\talice", "#chat", "MODE #other +o alice"},
		{"op", "alice", "#chat", "MODE #chat +o alice"},
		{"deop", "#other alice", "#chat", "MODE #other -o alice"},
		{"voice", "alice", "#chat", "MODE #chat +v alice"},
		{"devoice", "alice", "#chat", "MODE #chat -v alice"},
		{"kick", "alice please leave", "#chat", "KICK #chat alice :please leave"},
		{"kick", "#other alice :reason", "#chat", "KICK #other alice ::reason"},
		{"ban", "*!*@example.org", "#chat", "MODE #chat +b *!*@example.org"},
		{"ban", "", "#chat", "MODE #chat +b"},
		{"unban", "*!*@example.org", "#chat", "MODE #chat -b *!*@example.org"},
		{"invite", "alice", "#chat", "INVITE alice #chat"},
		{"invite", "alice #other", "*server*", "INVITE alice #other"},
		{"invite", "#other alice", "*server*", "INVITE alice #other"},
		{"list", "#chat", "*server*", "LIST #chat"},
		{"names", "#other", "*server*", "NAMES #other"},
		{"names", "", "#chat", "NAMES #chat"},
		{"who", "", "#chat", "WHO #chat"},
		{"who", "", "*server*", "WHO *"},
		{"who", "#other %tcuhsnfar,42", "#chat", "WHO #other %tcuhsnfar,42"},
		{"whowas", "alice 2 irc.example.org", "*server*", "WHOWAS alice 2 irc.example.org"},
		{"motd", "", "*server*", "MOTD"},
		{"time", "irc.example.org", "*server*", "TIME irc.example.org"},
		{"stats", "u", "*server*", "STATS u"},
		{"links", "irc.example.org *.example.org", "*server*", "LINKS irc.example.org *.example.org"},
	}
	for _, tt := range tests {
		t.Run(tt.cmd+" "+tt.args+" in "+tt.target, func(t *testing.T) {
			event, err := ircCommand(tt.cmd, tt.args, tt.target, "tester")
			if err != nil {
				t.Fatal(err)
			}
			if got := event.String(); got != tt.wire {
				t.Fatalf("wire = %q, want %q", got, tt.wire)
			}
			// Exercise the same serialization/parsing boundary as Backend.Raw.
			parsed := girc.ParseEvent(event.String())
			if parsed == nil || !slices.Equal(parsed.Params, event.Params) {
				t.Fatalf("parameters changed in transit: %#v -> %#v", event, parsed)
			}
		})
	}
}

func TestIRCCommandRejectsInvalidArguments(t *testing.T) {
	for _, tt := range []struct{ cmd, args, target string }{
		{"oper", "admin", "#chat"}, {"oper", ":admin secret", "#chat"},
		{"oper", "admin secret\r\nQUIT", "#chat"}, {"oper", "admin secret\x00", "#chat"},
		{"op", "", "#chat"}, {"op", "alice bob", "#chat"},
		{"kick", "alice", "*server*"}, {"unban", "", "#chat"},
		{"back", "extra", "#chat"}, {"away", "hello\r\nOPER x y", "#chat"},
		{"invite", "alice bob", "#chat"}, {"invite", "alice", "*server*"},
		{"names", "", "*server*"}, {"names", "#chat extra", "#chat"},
		{"mode", "#chat :+o alice", "#chat"}, {"time", "server extra", "#chat"},
		{"whowas", "", "#chat"}, {"whowas", "alice nope", "#chat"},
		{"away", strings.Repeat("x", 511), "#chat"},
	} {
		event, err := ircCommand(tt.cmd, tt.args, tt.target, "tester")
		if event != nil || err == nil {
			t.Errorf("/%s accepted invalid arguments", tt.cmd)
		}
	}
}

type slashBackend struct {
	ircclient.Backend
	server, line string
	err          error
	eventSink    func(ircclient.Event)
	reply        func()
}

func (b *slashBackend) SetMessageSink(func(model.Message))      {}
func (b *slashBackend) SetEventSink(sink func(ircclient.Event)) { b.eventSink = sink }
func (b *slashBackend) SetUpdateSink(func())                    {}
func (b *slashBackend) ServerNames() []string                   { return []string{"test"} }
func (b *slashBackend) KnownTargets(string) []string            { return []string{"#chat"} }
func (b *slashBackend) CurrentNick(string) string               { return "tester" }
func (b *slashBackend) Raw(server, line string) error {
	b.server, b.line = server, line
	if b.reply != nil {
		b.reply()
	}
	return b.err
}
func newSlashApp() (*App, *slashBackend) {
	disabled := false
	b := &slashBackend{}
	app := NewWithBackend(&config.Config{
		General:   config.GeneralConfig{HistoryLines: 100, Logging: &disabled},
		Scripting: config.ScriptingConfig{Enabled: &disabled},
	}, b)
	app.state.Select("test", "#chat")
	return app, b
}

func TestSlashCommandDispatchAndErrors(t *testing.T) {
	app, backend := newSlashApp()
	app.execute("/MoDe +m")
	if backend.server != "test" || backend.line != "MODE #chat +m" {
		t.Fatalf("sent %q/%q", backend.server, backend.line)
	}
	backend.line = ""
	app.execute("/invite")
	if backend.line != "" {
		t.Fatal("invalid command reached backend")
	}
	backend.err = errors.New("invalid event: OPER admin :test secret")
	app.execute("/oper admin test secret")
	for _, msg := range app.state.Current().Messages {
		if strings.Contains(msg.Text, "test secret") {
			t.Fatal("OPER error disclosed password")
		}
	}
}

func TestExplicitQueriesDisplayOtherwiseHiddenReplies(t *testing.T) {
	for _, tt := range []struct {
		line    string
		replies []ircclient.Event
		want    []string
	}{
		{"/names", []ircclient.Event{
			{Command: "353", Params: []string{"tester", "=", "#chat", "@alice bob"}},
			{Command: "366", Params: []string{"tester", "#chat", "End"}},
		}, []string{"Names for #chat: @alice bob", "End of NAMES for #chat"}},
		{"/who", []ircclient.Event{
			{Command: "352", Params: []string{"tester", "#chat", "user", "host", "server", "alice", "H", "0 Alice"}},
			{Command: "315", Params: []string{"tester", "#chat", "End"}},
		}, []string{"WHO: #chat user host server alice H 0 Alice", "End of WHO for #chat"}},
		{"/mode", []ircclient.Event{
			{Command: "324", Params: []string{"tester", "#chat", "+nt"}},
			{Command: "329", Params: []string{"tester", "#chat", "1234567890"}},
		}, []string{"Modes for #chat: +nt", "Channel created (Unix time): 1234567890"}},
	} {
		t.Run(tt.line, func(t *testing.T) {
			app, backend := newSlashApp()
			emit := func() {
				for _, ev := range tt.replies {
					ev.Server = "test"
					backend.eventSink(ev)
				}
			}
			// The same automatic replies remain quiet when no command is pending.
			emit()
			if len(app.state.Current().Messages) != 0 {
				t.Fatal("unsolicited housekeeping appeared")
			}
			// Simulate a fast backend replying before Raw returns, after a buffer switch.
			backend.reply = func() { app.state.Select("test", "*server*"); emit() }
			app.execute(tt.line)
			buffer := app.state.Find("test", "#chat")
			var got []string
			for _, msg := range buffer.Messages {
				got = append(got, msg.Text)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("reply output = %#v, want %#v", got, tt.want)
			}
			if len(app.pendingIRCQueries) != 0 {
				t.Fatal("completed query retained")
			}
			emit()
			if len(app.state.Find("test", "#chat").Messages) != len(tt.want) {
				t.Fatal("duplicate unsolicited output")
			}
		})
	}
}

func TestIRCQueryCleanupAndServerIsolation(t *testing.T) {
	app, backend := newSlashApp()
	backend.err = errors.New("offline")
	app.execute("/names")
	if len(app.pendingIRCQueries) != 0 {
		t.Fatal("failed send retained a query")
	}
	backend.err = nil
	app.execute("/names")
	reply := ircclient.Event{Server: "other", Command: "366", Params: []string{"tester", "#chat", "End"}}
	backend.eventSink(reply)
	if len(app.pendingIRCQueries) != 1 {
		t.Fatal("another server consumed query")
	}
	app.pendingIRCQueries[0].expires = time.Now().Add(-time.Second)
	reply.Server = "test"
	before := len(app.state.Current().Messages)
	backend.eventSink(reply)
	if len(app.pendingIRCQueries) != 0 || len(app.state.Current().Messages) != before {
		t.Fatal("expired query produced output")
	}
	app.execute("/names")
	app.handleIRCQueryReply(ircclient.Event{Server: "test", Command: "403", Params: []string{"tester", "#chat", "No such channel"}})
	if len(app.pendingIRCQueries) != 0 {
		t.Fatal("server error retained query")
	}
	app.execute("/names")
	app.handleIRCQueryReply(ircclient.Event{Server: "test", Command: ircclient.EventDisconnected})
	if len(app.pendingIRCQueries) != 0 {
		t.Fatal("disconnect retained query")
	}
	app.execute("/names")
	app.bindBackend(backend)
	if len(app.pendingIRCQueries) != 0 {
		t.Fatal("backend replacement retained query")
	}
}

func TestOperHistoryClearsNavigationDraft(t *testing.T) {
	app := newInputHistoryTestApp()
	app.addInputHistory("old message")
	app.input.Text = "/oper admin secret"
	app.inputHistoryPrevious()
	app.addInputHistory("/OpEr admin secret")
	h := app.inputHistoryCurrent()
	if len(h.entries) != 1 || h.draft != "" || h.active {
		t.Fatal("OPER retained in history or draft")
	}
	app.input.Text = ""
	app.inputHistoryNext()
	if app.input.Text != "" {
		t.Fatal("history recalled OPER password")
	}
}

package tui

import (
	"testing"
	"time"

	"copperline/internal/config"
	"copperline/internal/logging"
	"copperline/internal/model"
	ui "github.com/metaspartan/gotui/v5"
	"github.com/metaspartan/gotui/v5/widgets"
)

type blockedTypingBackend struct {
	failingSendBackend
	started chan struct{}
	release chan struct{}
}

func (b *blockedTypingBackend) SendTyping(string, string, string) (bool, error) {
	close(b.started)
	<-b.release
	return true, nil
}

func TestStalledTypingRequestDoesNotBlockKeyboard(t *testing.T) {
	backend := &blockedTypingBackend{started: make(chan struct{}), release: make(chan struct{})}
	defer close(backend.release)
	state := model.New(100)
	state.Select("testnet", "#chat")
	app := &App{
		cfg:   &config.Config{Relay: config.RelayConfig{Mode: "client"}},
		state: state, irc: backend, input: widgets.NewInput(),
		logger:       logging.New(false, "", "2006-01-02 15:04:05"),
		redraw:       make(chan struct{}, 1),
		transcript:   &transcriptList{List: widgets.NewList()},
		inputHistory: make(map[string]*inputHistoryState),
	}
	finished := make(chan struct{})
	go func() {
		app.handleKey(ui.Event{Type: ui.KeyboardEvent, ID: "a"})
		app.handleKey(ui.Event{Type: ui.KeyboardEvent, ID: "b"})
		app.handleKey(ui.Event{Type: ui.KeyboardEvent, ID: "c"})
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("keyboard waited for a typing notification")
	}
	select {
	case <-backend.started:
	case <-time.After(time.Second):
		t.Fatal("typing request not started")
	}
	if app.input.Text != "abc" {
		t.Fatalf("typed draft = %q", app.input.Text)
	}
}

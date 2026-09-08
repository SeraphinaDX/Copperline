package tui

import (
	"fmt"
	"time"

	ircclient "copperline/internal/irc"
	"copperline/internal/model"
)

// SetRelayReconnectFactory enables the relay-client reconnect UI. The factory
// creates a fresh SSH-backed backend; it is deliberately supplied by main so
// the TUI remains transport-agnostic.
func (a *App) SetRelayReconnectFactory(factory func() (ircclient.Backend, error)) {
	a.relayReconnect = factory
}

func (a *App) relayReconnectEnabled() bool {
	return a.cfg != nil && a.cfg.Relay.ModeValue() == "client" && a.relayReconnect != nil
}

func (a *App) requestRelayReconnect() {
	if !a.relayReconnectEnabled() || !a.relayReconnecting.CompareAndSwap(false, true) {
		return
	}

	// Detach the old SSH client first: a force reconnect must not leave a stale
	// transport alive while a second attachment is created. Clear callbacks so
	// shutdown cannot race a stale backend update into the TUI.
	old := a.irc
	old.SetMessageSink(nil)
	old.SetEventSink(nil)
	old.SetUpdateSink(nil)
	old.Stop("Copperline relay force reconnect")
	a.requestRedraw()

	go func() {
		backend, err := a.relayReconnect()
		a.relayReconnectDone <- relayReconnectResult{backend: backend, err: err}
	}()
}

func (a *App) finishRelayReconnect(result relayReconnectResult) {
	defer a.relayReconnecting.Store(false)
	if result.err != nil {
		if b := a.state.CurrentInfo(); b != nil {
			a.onMessage(model.Message{Time: time.Now(), Server: b.Server, Target: b.Target, Kind: model.KindError, Text: "Relay reconnect failed: " + result.err.Error()})
		}
		a.requestRedraw()
		return
	}
	if result.backend == nil {
		return
	}

	a.irc = result.backend
	a.bindBackend(result.backend)
	result.backend.Start()
	a.onBackendUpdate()
	if b := a.state.CurrentInfo(); b != nil {
		a.onMessage(model.Message{Time: time.Now(), Server: b.Server, Target: b.Target, Kind: model.KindSystem, Text: "Relay SSH connection re-established."})
	}
}

func (a *App) relayReconnectLabel() string {
	if !a.relayReconnectEnabled() {
		return ""
	}
	if a.relayReconnecting.Load() {
		return "[⟳ RECONNECTING…]"
	}
	if !a.relayTransportConnected() {
		return fmt.Sprintf("[⚠ RECONNECT (%s)]", a.cfg.Keybindings.RelayReconnect)
	}
	return fmt.Sprintf("[⟳ RECONNECT (%s)]", a.cfg.Keybindings.RelayReconnect)
}

// relayTransportConnected asks relay backends about the SSH attachment itself.
// IRC connection state is deliberately separate: a relay can be perfectly
// reachable while one IRC network is disconnected, and the inverse used to be
// the dangerous case where a cached IRC snapshot made a dead SSH link look live.
func (a *App) relayTransportConnected() bool {
	if a.cfg == nil || a.cfg.Relay.ModeValue() != "client" {
		return true
	}
	type transportHealth interface {
		TransportConnected() bool
	}
	health, ok := a.irc.(transportHealth)
	return !ok || health.TransportConnected()
}

func (a *App) relayReconnectControlWidth() int {
	label := a.relayReconnectLabel()
	if label == "" {
		return 0
	}
	return len([]rune(label)) + 2
}

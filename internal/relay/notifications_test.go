package relay

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"copperline/internal/config"
	"copperline/internal/gotify"
	ircclient "copperline/internal/irc"
	"copperline/internal/model"
)

func TestClientNotificationOwnershipWhenRelayDisabled(t *testing.T) {
	requests := make(chan struct{}, 4)
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests <- struct{}{}; w.WriteHeader(200) }))
	defer httpServer.Close()
	cfg := &config.Config{General: config.GeneralConfig{Nick: "me"}, Gotify: config.GotifyConfig{Enabled: false, URL: httpServer.URL, TimeoutSeconds: 1}, Servers: []config.ServerConfig{{Name: "test", Nick: "me"}}}
	s := &Server{cfg: cfg, irc: ircclient.New(cfg, nil), notifier: gotify.New(cfg.Gotify), peers: make(map[*serverPeer]struct{})}
	peer := func(enabled bool) *serverPeer {
		return &serverPeer{server: s, notifications: enabled, sendQ: make(chan frame, 8), closed: make(chan struct{})}
	}
	a, b, c := peer(true), peer(true), peer(false)
	s.addPeer(a)
	s.addPeer(b)
	s.addPeer(c)
	msg := model.Message{Server: "test", Target: "alice", Nick: "alice", Text: "hello", Kind: model.KindMessage}
	s.deliverMessage(msg, "me")
	if (<-a.sendQ).Message.SuppressNotify || !(<-b.sendQ).Message.SuppressNotify || !(<-c.sendQ).Message.SuppressNotify {
		t.Fatal("expected only oldest eligible client to notify")
	}
	s.removePeer(a)
	s.deliverMessage(msg, "me")
	if (<-b.sendQ).Message.SuppressNotify || !(<-c.sendQ).Message.SuppressNotify {
		t.Fatal("notification ownership did not transfer")
	}
	select {
	case <-requests:
		t.Fatal("relay notified while attached")
	default:
	}
	s.removePeer(b)
	s.removePeer(c)
	s.deliverMessage(msg, "me")
	select {
	case <-requests:
		t.Fatal("disabled relay notified")
	default:
	}
}

func TestRelayNotificationsContinueAcrossClientAttachments(t *testing.T) {
	requests := make(chan struct{}, 16)
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests <- struct{}{}; w.WriteHeader(200) }))
	defer endpoint.Close()
	cfg := &config.Config{Gotify: config.GotifyConfig{Enabled: true, URL: endpoint.URL, TimeoutSeconds: 1}}
	s := &Server{cfg: cfg, notifier: gotify.New(cfg.Gotify), peers: make(map[*serverPeer]struct{})}
	msg := model.Message{Server: "test", Target: "#chat", Nick: "alice", Text: "hello me", Kind: model.KindMessage}
	check := func() {
		t.Helper()
		s.deliverMessage(msg, "me")
		select {
		case <-requests:
		case <-time.After(time.Second):
			t.Fatal("relay mention alert missing")
		}
		for p := range s.peers {
			if !(<-p.sendQ).Message.SuppressNotify {
				t.Fatal("client could duplicate relay alert")
			}
		}
	}
	check() // initially unattended
	desktop := &serverPeer{server: s, sendQ: make(chan frame, 8), closed: make(chan struct{})}
	s.addPeer(desktop)
	check() // desktop has Gotify disabled
	laptop := &serverPeer{server: s, notifications: true, sendQ: make(chan frame, 8), closed: make(chan struct{})}
	s.addPeer(laptop)
	check() // notification-capable client cannot take ownership from relay
	s.removePeer(laptop)
	check()
	s.removePeer(desktop)
	check()
}

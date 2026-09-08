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

func TestNotificationOwnershipAndDetach(t *testing.T) {
	requests := make(chan struct{}, 4)
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests <- struct{}{}; w.WriteHeader(200) }))
	defer httpServer.Close()
	cfg := &config.Config{General: config.GeneralConfig{Nick: "me"}, Gotify: config.GotifyConfig{Enabled: true, URL: httpServer.URL, TimeoutSeconds: 1}, Servers: []config.ServerConfig{{Name: "test", Nick: "me"}}}
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
	case <-time.After(2 * time.Second):
		t.Fatal("missing detached relay notification")
	}
}

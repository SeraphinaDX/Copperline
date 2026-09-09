package relay

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"copperline/internal/config"
	"copperline/internal/model"
)

func TestSSHReplayAfterRestart(t *testing.T) {
	dir := t.TempDir()
	no := false
	cfg := &config.Config{
		General: config.GeneralConfig{HistoryLines: 10, Logging: &no},
		Relay:   config.RelayConfig{Mode: "server", User: "copperline", HostKey: filepath.Join(dir, "host"), AuthorizedKeys: filepath.Join(dir, "authorized"), PrivateKey: filepath.Join(dir, "client"), HistoryFile: filepath.Join(dir, "history.json")},
	}
	_, _, pub, err := loadOrCreateClientSigner(cfg.Relay.PrivateKey, "")
	if err != nil {
		t.Fatal(err)
	}
	key, err := os.ReadFile(pub)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.Relay.AuthorizedKeys, key, 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := NewServer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	s.onMessage(model.Message{Server: "test", Target: "#go", Nick: "alice", Text: "before restart", Kind: model.KindMessage, Time: time.Now()})
	id := s.replayMessages()[0].RelayID
	if err := s.saveHistory(); err != nil {
		t.Fatal(err)
	}
	s, err = NewServer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close(); s.closePeers() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serveConn(conn)
		}
	}()
	clientCfg := *cfg
	clientCfg.Relay.Mode = "client"
	clientCfg.Relay.Address = ln.Addr().String()
	clientCfg.Relay.HostKeyFingerprint = s.HostKeyFingerprint()
	connect := func() (*Client, chan model.Message) {
		c, err := NewClient(&clientCfg)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { c.Stop("") })
		messages := make(chan model.Message, 10)
		c.SetMessageSink(func(msg model.Message) { messages <- msg })
		return c, messages
	}
	receive := func(messages chan model.Message) model.Message {
		select {
		case msg := <-messages:
			return msg
		case <-time.After(2 * time.Second):
			t.Fatal("no SSH message")
			return model.Message{}
		}
	}
	c, messages := connect()
	status, err := c.GotifyStatus()
	if err != nil || !strings.Contains(status, "Relay Gotify: disabled") {
		t.Fatalf("remote Gotify status=%q err=%v", status, err)
	}
	if err := c.GotifyTest(); err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("remote Gotify test error=%v", err)
	}

	msg := receive(messages)
	if msg.RelayID != id || !msg.Replay {
		t.Fatalf("restored SSH replay=%#v", msg)
	}
	s.onMessage(model.Message{Server: "test", Target: "#go", Nick: "alice", Text: "live", Kind: model.KindMessage, Time: time.Now()})
	live := receive(messages)
	if live.Replay || live.RelayID == "" || live.RelayID == id {
		t.Fatalf("live message=%#v", live)
	}
	c.Stop("")
	_, replay := connect()
	if receive(replay).RelayID != id || receive(replay).RelayID != live.RelayID {
		t.Fatal("reconnect changed message identities")
	}
}

package relay

import (
	"encoding/json"
	"net"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

type stalledCloseChannel struct {
	ssh.Channel
	started chan struct{}
	release chan struct{}
}

func (c *stalledCloseChannel) Close() error {
	close(c.started)
	<-c.release
	return nil
}

func TestSleepingPeerCannotBlockOtherClients(t *testing.T) {
	s := &Server{peers: make(map[*serverPeer]struct{})}
	ch := &stalledCloseChannel{started: make(chan struct{}), release: make(chan struct{})}
	defer close(ch.release)
	sleeping := &serverPeer{server: s, ch: ch, sendQ: make(chan frame, 1), closed: make(chan struct{})}
	desktop := &serverPeer{server: s, sendQ: make(chan frame, 4), closed: make(chan struct{})}
	s.addPeer(sleeping)
	s.addPeer(desktop)
	// The laptop stops consuming traffic; its bounded queue eventually fills.
	sleeping.sendQ <- frame{Type: "snapshot"}
	finished := make(chan struct{})
	go func() { s.broadcast(frame{Type: "event"}); close(finished) }()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("sleeping laptop blocked shared broadcast")
	}
	select {
	case <-ch.started:
	case <-time.After(time.Second):
		t.Fatal("sleeping peer cleanup did not start")
	}
	// Repeated cleanup must not wait on the first blocked SSH close either.
	again := make(chan struct{})
	go func() { sleeping.close(); s.broadcast(frame{Type: "response", ID: 42}); close(again) }()
	select {
	case <-again:
	case <-time.After(time.Second):
		t.Fatal("cleanup stalled subsequent desktop traffic")
	}
	if f := <-desktop.sendQ; f.Type != "event" {
		t.Fatalf("desktop event = %#v", f)
	}
	if f := <-desktop.sendQ; f.ID != 42 {
		t.Fatalf("desktop response = %#v", f)
	}
	s.peersMu.Lock()
	defer s.peersMu.Unlock()
	if _, ok := s.peers[sleeping]; ok {
		t.Fatal("sleeping laptop still attached")
	}
	if _, ok := s.peers[desktop]; !ok {
		t.Fatal("healthy desktop detached")
	}
}

func TestStalledPeerWriteClosesOnlyItsTransport(t *testing.T) {
	local, remote := net.Pipe()
	defer remote.Close()
	s := &Server{peers: make(map[*serverPeer]struct{})}
	p := &serverPeer{server: s, transport: local, enc: json.NewEncoder(local), closed: make(chan struct{})}
	desktop := &serverPeer{server: s, sendQ: make(chan frame, 1), closed: make(chan struct{})}
	s.addPeer(p)
	s.addPeer(desktop)
	finished := make(chan error, 1)
	go func() { finished <- p.writeFrame(frame{Type: "snapshot"}, 50*time.Millisecond) }()
	select {
	case err := <-finished:
		if err == nil {
			t.Fatal("stalled write succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("stalled SSH writer not released")
	}
	s.broadcast(frame{Type: "response", ID: 7})
	if f := <-desktop.sendQ; f.ID != 7 {
		t.Fatalf("healthy desktop response = %#v", f)
	}
}

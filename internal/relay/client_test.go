package relay

import (
	"context"
	"copperline/internal/config"
	"copperline/internal/model"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"golang.org/x/crypto/ssh"
	"net"
	"testing"
	"time"
)

// A pipe with no reader reproduces a write stuck after a machine sleeps.
// The timeout must cover that write and requests queued behind it.
func TestRequestTimeoutIncludesBlockedWrite(t *testing.T) {
	local, remote := net.Pipe()
	defer remote.Close()
	c := &Client{transport: local, enc: json.NewEncoder(local), done: make(chan struct{}), pending: make(map[uint64]chan frame), connected: true}
	results := make(chan error, 2)
	for range 2 {
		go func() {
			_, err := c.callWithTimeout("message", frame{Text: "retain me"}, 50*time.Millisecond)
			results <- err
		}()
	}
	for range 2 {
		select {
		case err := <-results:
			if err == nil {
				t.Fatal("blocked send reported success")
			}
		case <-time.After(time.Second):
			t.Fatal("request stuck before response timeout")
		}
	}
	if c.TransportConnected() {
		t.Fatal("stale connection remains connected")
	}
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()
	if len(c.pending) != 0 {
		t.Fatal("pending requests leaked")
	}
}

func TestOptionsCancelSSHHandshake(t *testing.T) {
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		c, err := listener.Accept()
		if err == nil {
			accepted <- c
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := NewClientWithOptions(&config.Config{Relay: config.RelayConfig{Address: listener.Addr().String(), User: "test"}}, ClientOptions{Signer: signer, Context: ctx})
		result <- err
	}()
	var transport net.Conn
	select {
	case transport = <-accepted:
	case <-time.After(time.Second):
		t.Fatal("no dial")
	}
	defer transport.Close()
	cancel()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("cancelled handshake succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation did not unblock SSH")
	}
}

func TestLiveMessagesBeforeSinkAreRetained(t *testing.T) {
	local, remote := net.Pipe()
	defer remote.Close()
	c := &Client{transport: local, dec: json.NewDecoder(local), done: make(chan struct{}), pending: make(map[uint64]chan frame), connected: true}
	defer c.Stop("")
	go c.reader()
	msg := model.Message{RelayID: "live-1", Server: "test", Target: "#go", Text: "arrived during setup"}
	if err := json.NewEncoder(remote).Encode(frame{Type: "message", Message: &msg}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		c.mu.RLock()
		count := len(c.pendingHistory)
		c.mu.RUnlock()
		if count == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("message lost before sink installation")
		}
		time.Sleep(time.Millisecond)
	}
	received := make(chan model.Message, 1)
	c.SetMessageSink(func(m model.Message) { received <- m })
	select {
	case got := <-received:
		if got.RelayID != msg.RelayID || got.Replay {
			t.Fatalf("live message changed: %#v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("queued message not delivered")
	}
}

func TestReplayDrainsBeforeConcurrentLiveMessage(t *testing.T) {
	local, remote := net.Pipe()
	defer remote.Close()
	c := &Client{transport: local, dec: json.NewDecoder(local), done: make(chan struct{}), pending: make(map[uint64]chan frame), connected: true,
		pendingHistory: []model.Message{{Text: "history-1", Replay: true}, {Text: "history-2", Replay: true}}}
	defer c.Stop("")
	go c.reader()
	first := make(chan struct{})
	release := make(chan struct{})
	messages := make(chan string, 3)
	go c.SetMessageSink(func(msg model.Message) {
		if msg.Text == "history-1" {
			close(first)
			<-release
		}
		messages <- msg.Text
	})
	select {
	case <-first:
	case <-time.After(time.Second):
		t.Fatal("history not draining")
	}
	live := model.Message{Text: "live"}
	if err := json.NewEncoder(remote).Encode(frame{Type: "message", Message: &live}); err != nil {
		t.Fatal(err)
	}
	close(release)
	for _, want := range []string{"history-1", "history-2", "live"} {
		select {
		case got := <-messages:
			if got != want {
				t.Fatalf("message=%q want=%q", got, want)
			}
		case <-time.After(time.Second):
			t.Fatal("delivery stalled")
		}
	}
}

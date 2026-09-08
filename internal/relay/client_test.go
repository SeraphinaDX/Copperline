package relay

import (
	"encoding/json"
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

package irc

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"copperline/internal/config"
	"github.com/lrstanley/girc"
)

func mockIRC(t *testing.T, c *girc.Client, reply string) <-chan string {
	t.Helper()
	client, server := net.Pipe()
	lines := make(chan string, 32)
	ready := make(chan struct{})
	finished := make(chan struct{})
	go func() { defer close(finished); _ = c.MockConnect(client) }()
	go func() {
		scan := bufio.NewScanner(server)
		for scan.Scan() {
			line := scan.Text()
			lines <- line
			if strings.HasPrefix(line, "USER ") {
				close(ready)
			}
			e := girc.ParseEvent(line)
			if e != nil && e.Command == girc.PING && len(e.Params) > 0 {
				switch reply {
				case "pong":
					fmt.Fprintf(server, ":irc.test PONG irc.test :%s\r\n", e.Params[0])
				case "wrong":
					fmt.Fprint(server, ":irc.test PONG irc.test :unrelated\r\n")
				case "disconnect":
					server.Close()
				}
			}
		}
	}()
	t.Cleanup(func() {
		c.Close()
		client.Close()
		server.Close()
		select {
		case <-finished:
		case <-time.After(time.Second):
			t.Error("IRC did not stop")
		}
	})
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("IRC did not connect")
	}
	return lines
}

func TestSendRequiresMatchingServerRoundTrip(t *testing.T) {
	for _, mode := range []string{"pong", "wrong", "silent", "disconnect"} {
		t.Run(mode, func(t *testing.T) {
			c := girc.New(girc.Config{Server: "irc.test", Nick: "tester", User: "tester", AllowFlood: true})
			lines := mockIRC(t, c, mode)
			err := confirmSend(c, func() { c.Cmd.Message("#chat", "keep this text") }, 100*time.Millisecond)
			if (err == nil) != (mode == "pong") {
				t.Fatalf("confirmation error = %v", err)
			}
			sawMessage := false
			for len(lines) > 0 {
				line := <-lines
				if strings.HasPrefix(line, "PRIVMSG ") {
					sawMessage = true
				}
				if strings.HasPrefix(line, "PING ") && !sawMessage {
					t.Fatal("confirmation sent before message")
				}
			}
			if !sawMessage {
				t.Fatal("message never reached mock server")
			}
		})
	}
}

func TestTCPConnectionIsNotIRCReady(t *testing.T) {
	m := New(&config.Config{Servers: []config.ServerConfig{{Name: "test", Host: "irc.test", Nick: "tester", User: "tester"}}}, nil)
	c := m.sessions["test"].client
	mockIRC(t, c, "pong")
	if !c.IsConnected() {
		t.Fatal("mock TCP is not connected")
	}
	if m.IsConnected("test") {
		t.Fatal("unregistered TCP connection advertised as ready")
	}
	if err := m.SendMessage("test", "#chat", "too early"); err == nil {
		t.Fatal("sent before IRC registration")
	}
	c.RunHandlers(&girc.Event{Command: girc.CONNECTED})
	if !m.IsConnected("test") {
		t.Fatal("registered connection not ready")
	}
	if err := m.SendMessage("test", "#chat", "ready now"); err != nil {
		t.Fatal(err)
	}
	c.RunHandlers(&girc.Event{Command: girc.DISCONNECTED})
	if m.IsConnected("test") {
		t.Fatal("disconnected connection still ready")
	}
}

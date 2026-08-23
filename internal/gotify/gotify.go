package gotify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"copperline/internal/config"
)

// Notifier sends Copperline notifications to a Gotify application. Normal
// notifications are queued and sent by a background worker so Gotify network
// latency never blocks IRC processing or the TUI.
type Notifier struct {
	enabled  bool
	endpoint string
	token    string
	priority int
	client   *http.Client
	queue    chan message
}

type message struct {
	Title    string `json:"title,omitempty"`
	Message  string `json:"message"`
	Priority int    `json:"priority"`
}

func New(cfg config.GotifyConfig) *Notifier {
	n := &Notifier{enabled: cfg.Enabled}
	if !cfg.Enabled {
		return n
	}

	n.endpoint = messageEndpoint(cfg.URL)
	n.token = config.Secret(cfg.Token, cfg.TokenEnv)
	n.priority = cfg.PriorityValue()
	n.client = &http.Client{Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second}
	n.queue = make(chan message, 64)
	go n.worker()
	return n
}

func (n *Notifier) Enabled() bool { return n != nil && n.enabled }

// Send queues a Gotify notification. The queue is deliberately bounded; if
// Gotify is down for a long time, IRC processing must continue rather than
// accumulating unbounded notification work.
func (n *Notifier) Send(title, body string) {
	if !n.Enabled() || strings.TrimSpace(body) == "" {
		return
	}
	msg := message{Title: title, Message: body, Priority: n.priority}
	select {
	case n.queue <- msg:
	default:
		// Drop rather than blocking the IRC/TUI path when the notification queue
		// is full. A manual /gotify test can be used to diagnose the endpoint.
	}
}

// Test performs a real Gotify request and returns the server/network error to
// the caller. It is intended to be invoked from a goroutine by the TUI.
func (n *Notifier) Test(ctx context.Context) error {
	if !n.Enabled() {
		return fmt.Errorf("Gotify is disabled")
	}
	return n.post(ctx, message{
		Title:    "Copperline test",
		Message:  "Gotify notifications are working.",
		Priority: n.priority,
	})
}

func (n *Notifier) worker() {
	for msg := range n.queue {
		ctx, cancel := context.WithTimeout(context.Background(), n.client.Timeout)
		_ = n.post(ctx, msg)
		cancel()
	}
}

func (n *Notifier) post(ctx context.Context, msg message) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Gotify-Key", n.token)

	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		limited, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		text := strings.TrimSpace(string(limited))
		if text == "" {
			return fmt.Errorf("Gotify returned %s", resp.Status)
		}
		return fmt.Errorf("Gotify returned %s: %s", resp.Status, text)
	}
	return nil
}

func messageEndpoint(base string) string {
	base = strings.TrimSpace(base)
	base = strings.TrimRight(base, "/")
	if strings.HasSuffix(strings.ToLower(base), "/message") {
		return base
	}
	return base + "/message"
}

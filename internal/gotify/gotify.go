package gotify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"copperline/internal/config"
)

// Notifier sends Copperline notifications to a Gotify application. Normal
// notifications are queued and sent by a background worker so Gotify network
// latency never blocks IRC processing or the TUI.
type Notifier struct {
	enabled                  bool
	endpoint                 string
	token                    string
	priority                 int
	client                   *http.Client
	queue                    chan message
	mu                       sync.Mutex
	sent, failed, dropped    uint64
	lastAttempt, lastSuccess time.Time
	lastError                string
}

type message struct {
	Title    string `json:"title,omitempty"`
	Message  string `json:"message"`
	Priority int    `json:"priority"`
}

func New(cfg config.GotifyConfig) *Notifier {
	n := &Notifier{enabled: cfg.Enabled, lastError: "none"}
	if !cfg.Enabled {
		return n
	}

	n.endpoint = messageEndpoint(cfg.URL)
	n.token = config.Secret(cfg.Token, cfg.TokenEnv)
	n.priority = cfg.PriorityValue()
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	n.client = &http.Client{Timeout: timeout}
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
		n.mu.Lock()
		n.dropped++
		n.mu.Unlock()
	}
}

// Test performs a real Gotify request and returns the server/network error to
// the caller. It is intended to be invoked from a goroutine by the TUI.
func (n *Notifier) Test(ctx context.Context) error {
	if !n.Enabled() {
		return fmt.Errorf("Gotify is disabled")
	}
	return n.deliver(ctx, message{
		Title:    "Copperline test",
		Message:  "Gotify notifications are working.",
		Priority: n.priority,
	})
}

func (n *Notifier) worker() {
	for msg := range n.queue {
		ctx, cancel := context.WithTimeout(context.Background(), n.client.Timeout)
		_ = n.deliver(ctx, msg)
		cancel()
	}
}

// Status reports delivery at the HTTP endpoint, not whether a phone displayed it.
func (n *Notifier) Status() string {
	if !n.Enabled() {
		return "disabled"
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	format := func(t time.Time) string {
		if t.IsZero() {
			return "never"
		}
		return t.UTC().Format(time.RFC3339)
	}
	return fmt.Sprintf("enabled; sent=%d failed=%d dropped=%d queued=%d; last attempt=%s; last success=%s; last error=%s", n.sent, n.failed, n.dropped, len(n.queue), format(n.lastAttempt), format(n.lastSuccess), n.lastError)
}

func (n *Notifier) deliver(ctx context.Context, msg message) error {
	n.mu.Lock()
	n.lastAttempt = time.Now()
	n.mu.Unlock()
	err := n.post(ctx, msg)
	n.mu.Lock()
	defer n.mu.Unlock()
	if err != nil {
		n.failed++
		// Do not expose URLs, tokens, response bodies, or message contents in status.
		n.lastError = "HTTP/network request failed"
		if e, ok := err.(httpStatusError); ok {
			n.lastError = e.Error()
		}
		if ctx.Err() != nil {
			n.lastError = ctx.Err().Error()
		}
	} else {
		n.sent++
		n.lastSuccess = time.Now()
		n.lastError = "none"
	}
	return err
}

type httpStatusError int

func (e httpStatusError) Error() string {
	return fmt.Sprintf("Gotify HTTP %d %s", int(e), http.StatusText(int(e)))
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
		return httpStatusError(resp.StatusCode)
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
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

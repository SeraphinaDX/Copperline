// Package urlgrab collects links without putting disk I/O on IRC delivery.
package urlgrab

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"copperline/internal/model"
)

type Record struct {
	Time   time.Time `json:"time"`
	Server string    `json:"server"`
	Target string    `json:"target"`
	Nick   string    `json:"nick"`
	URL    string    `json:"url"`
}

type Collector struct {
	mu       sync.Mutex
	closed   bool
	queue    chan []Record
	done     chan struct{}
	report   func(error)
	reported atomic.Bool
}

func New(path string, report func(error)) *Collector {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	c := &Collector{queue: make(chan []Record, 256), done: make(chan struct{}), report: report}
	go func() {
		defer close(c.done)
		seen, err := loadSaved(path)
		if err != nil {
			c.reportError(fmt.Errorf("read saved URLs (collection disabled): %w", err))
			return
		}
		for records := range c.queue {
			if err := appendRecords(path, records, seen); err != nil {
				c.reportError(err)
			}
		}
	}()
	return c
}

func (c *Collector) reportError(err error) {
	// A broken path or full queue must not flood the IRC server buffer.
	if c.reported.CompareAndSwap(false, true) && c.report != nil {
		go c.report(err)
	}
}

func (c *Collector) Add(msg model.Message) {
	if c == nil || msg.Replay || (msg.Kind != model.KindMessage && msg.Kind != model.KindAction && msg.Kind != model.KindNotice) {
		return
	}
	var records []Record
	for _, link := range Extract(model.PlainText(msg.Text)) {
		records = append(records, Record{msg.Time, msg.Server, msg.Target, msg.Nick, link})
	}
	if len(records) == 0 {
		return
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	select {
	case <-c.done:
		c.mu.Unlock()
	case c.queue <- records:
		c.mu.Unlock()
	default:
		c.mu.Unlock()
		c.reportError(fmt.Errorf("URL queue full; links dropped because storage is too slow"))
	}
}

func (c *Collector) Close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	if !c.closed {
		c.closed = true
		close(c.queue)
	}
	c.mu.Unlock()
	// Flush ordinary writes, but a stalled filesystem cannot trap shutdown.
	select {
	case <-c.done:
	case <-time.After(time.Second):
	}
}

// The worker alone owns this index. Read the existing file once so duplicate
// suppression survives restarts without scanning it for every incoming link.
func loadSaved(path string) (map[string]bool, error) {
	seen := make(map[string]bool)
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return seen, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	for {
		var record Record
		if err := dec.Decode(&record); err != nil {
			if err == io.EOF {
				return seen, nil
			}
			return nil, err
		}
		if record.URL == "" {
			return nil, fmt.Errorf("saved record has no URL")
		}
		seen[urlKey(record.URL)] = true
	}
}

func urlKey(link string) string {
	u, err := url.Parse(link)
	if err != nil {
		return link
	}
	u.Scheme, u.Host = strings.ToLower(u.Scheme), strings.ToLower(u.Host)
	return u.String()
}

func appendRecords(path string, records []Record, seen map[string]bool) error {
	pending := records[:0]
	for _, record := range records {
		if !seen[urlKey(record.URL)] {
			pending = append(pending, record)
		}
	}
	if len(pending) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	for _, record := range pending {
		key := urlKey(record.URL)
		if seen[key] {
			continue
		}
		if err := enc.Encode(record); err != nil {
			_ = f.Close()
			return err
		}
		seen[key] = true
	}
	return f.Close()
}

var links = regexp.MustCompile(`(?i)https?://[^\s<>"` + "`" + `]+`)

// Preserve balanced URL parentheses (common in wiki links), while stripping
// prose punctuation and surrounding delimiters. Never fetch or open a URL.
func Extract(text string) []string {
	var out []string
	seen := make(map[string]bool)
	for _, link := range links.FindAllString(text, -1) {
		link = strings.TrimRight(link, ".,;:!?'")
		for len(link) > 0 {
			last := link[len(link)-1]
			open := byte(0)
			switch last {
			case ')':
				open = '('
			case ']':
				open = '['
			case '}':
				open = '{'
			}
			if open == 0 || strings.Count(link, string(last)) <= strings.Count(link, string(open)) {
				break
			}
			link = strings.TrimRight(link[:len(link)-1], ".,;:!?'")
		}
		u, err := url.Parse(link)
		if err == nil && u.Hostname() != "" && !seen[link] {
			seen[link] = true
			out = append(out, link)
		}
	}
	return out
}

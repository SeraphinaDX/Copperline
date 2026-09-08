package model

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type Kind string

const (
	KindMessage Kind = "message"
	KindAction  Kind = "action"
	KindNotice  Kind = "notice"
	KindSystem  Kind = "system"
	KindError   Kind = "error"
	KindDCC     Kind = "dcc"
)

type Message struct {
	// RelayID is assigned once by the relay and survives history restoration.
	RelayID string `json:",omitempty"`
	// SuppressNotify is delivery metadata, never a change to message content.
	SuppressNotify bool `json:",omitempty"`
	Time           time.Time
	Server         string
	Target         string
	Nick           string
	Text           string
	Kind           Kind
	Tags           map[string]string
	Mention        bool
	// Replay marks messages restored from a Copperline relay's retained
	// history. The TUI displays them normally but does not re-log or
	// re-notify them on every client attachment.
	Replay bool
}

type Buffer struct {
	Server    string
	Target    string
	Messages  []Message
	Unread    int
	Total     uint64
	ReadTotal uint64
}

// BufferInfo is a lightweight buffer snapshot for UI/navigation code that
// does not need message history. Keeping these paths history-free avoids
// copying every message in every buffer during routine redraws.
type BufferInfo struct {
	Server    string
	Target    string
	Unread    int
	Total     uint64
	ReadTotal uint64
}

// MessageWindow describes the retained message window for the current buffer.
// Start is the number of older messages that have fallen out of the in-memory
// history and Total is the number of messages ever appended to this buffer.
// Messages contains only the portion requested by CurrentWindow.
type MessageWindow struct {
	BufferInfo
	Start    uint64
	Messages []Message
}

type State struct {
	mu         sync.RWMutex
	Buffers    map[string]*Buffer
	Order      []string
	CurrentKey string
	MaxLines   int
}

func New(maxLines int) *State {
	if maxLines <= 0 {
		maxLines = 1000
	}
	return &State{Buffers: make(map[string]*Buffer), MaxLines: maxLines}
}

func Key(server, target string) string {
	// IRC nicknames are case-insensitive. Keep channel/server buffer spelling
	// untouched, but canonicalize private-query targets so user-entered casing
	// (for example "Leah") and server-provided casing ("leah") resolve to the
	// same in-memory buffer.
	if isQueryTarget(target) {
		target = strings.ToLower(target)
	}
	return server + "\x00" + target
}

func isQueryTarget(target string) bool {
	return target != "" && target != "*server*" && !IsChannel(target)
}

func (s *State) Ensure(server, target string) *Buffer {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ensureLocked(server, target)
}

func (s *State) ensureLocked(server, target string) *Buffer {
	key := Key(server, target)
	if b := s.Buffers[key]; b != nil {
		return b
	}
	b := &Buffer{Server: server, Target: target}
	s.Buffers[key] = b
	s.Order = append(s.Order, key)
	if s.CurrentKey == "" {
		s.CurrentKey = key
	}
	return b
}

func (s *State) Add(msg Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := s.ensureLocked(msg.Server, msg.Target)
	// If a query was first opened with guessed/user-entered casing, adopt the
	// spelling seen on actual IRC traffic without creating a second buffer.
	// The map key remains the canonical case-insensitive query identity.
	if isQueryTarget(msg.Target) && b.Target != msg.Target {
		b.Target = msg.Target
	}
	b.Messages = append(b.Messages, msg)
	b.Total++
	if len(b.Messages) > s.MaxLines {
		drop := len(b.Messages) - s.MaxLines
		// Drop old messages in-place instead of allocating/copying the entire
		// retained history for every new line once the buffer is full. Clear
		// removed entries first so their strings/tags can be reclaimed.
		for i := 0; i < drop; i++ {
			b.Messages[i] = Message{}
		}
		b.Messages = b.Messages[drop:]
	}
	b.Unread++
}

func (s *State) Select(server, target string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := s.ensureLocked(server, target)
	b.Unread = 0
	b.ReadTotal = b.Total
	s.CurrentKey = Key(server, target)
}

// MarkReadThrough acknowledges only the displayed snapshot, leaving messages
// that arrived concurrently unread. Buffer selection separately acknowledges
// existing activity.
func (s *State) MarkReadThrough(server, target string, total uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := s.Buffers[Key(server, target)]
	if b == nil {
		return
	}
	if total > b.Total {
		total = b.Total
	}
	if total > b.ReadTotal {
		b.ReadTotal = total
	}
	b.Unread = int(b.Total - b.ReadTotal)
}

func (s *State) SelectKey(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.Buffers[key]; !ok {
		return false
	}
	s.CurrentKey = key
	s.Buffers[key].Unread = 0
	s.Buffers[key].ReadTotal = s.Buffers[key].Total
	return true
}

func (s *State) Current() *Buffer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b := s.Buffers[s.CurrentKey]
	if b == nil {
		return nil
	}
	return cloneBuffer(b)
}

// CurrentInfo returns the current buffer without copying its message history.
func (s *State) CurrentInfo() *BufferInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b := s.Buffers[s.CurrentKey]
	if b == nil {
		return nil
	}
	return &BufferInfo{Server: b.Server, Target: b.Target, Unread: b.Unread, Total: b.Total, ReadTotal: b.ReadTotal}
}

// SnapshotInfo returns lightweight buffer metadata in stable creation order.
// Unlike Snapshot, it never clones message slices.
func (s *State) SnapshotInfo() ([]BufferInfo, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]BufferInfo, 0, len(s.Order))
	for _, key := range s.Order {
		if b := s.Buffers[key]; b != nil {
			out = append(out, BufferInfo{Server: b.Server, Target: b.Target, Unread: b.Unread, Total: b.Total, ReadTotal: b.ReadTotal})
		}
	}
	return out, s.CurrentKey
}

// CurrentWindow returns only messages newer than sinceTotal when possible.
// If sinceTotal is older than the retained history window (or otherwise
// invalid), Messages contains the entire retained window so callers can reset
// their cache safely.
func (s *State) CurrentWindow(sinceTotal uint64) *MessageWindow {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b := s.Buffers[s.CurrentKey]
	if b == nil {
		return nil
	}
	start := b.Total - uint64(len(b.Messages))
	idx := 0
	if sinceTotal >= start && sinceTotal <= b.Total {
		idx = int(sinceTotal - start)
	}
	msgs := append([]Message(nil), b.Messages[idx:]...)
	return &MessageWindow{
		BufferInfo: BufferInfo{Server: b.Server, Target: b.Target, Unread: b.Unread, Total: b.Total, ReadTotal: b.ReadTotal},
		Start:      start,
		Messages:   msgs,
	}
}
func (s *State) Snapshot() ([]*Buffer, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Buffer, 0, len(s.Order))
	for _, key := range s.Order {
		if b := s.Buffers[key]; b != nil {
			out = append(out, cloneBuffer(b))
		}
	}
	return out, s.CurrentKey
}

func (s *State) Find(server, target string) *Buffer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneBuffer(s.Buffers[Key(server, target)])
}

// ContainsMessage reports whether the retained in-memory window already holds
// the same relay message. It is used to de-duplicate retained relay history
// after a forced SSH reconnect while still allowing messages received during
// the disconnected gap to be replayed.
func (s *State) ContainsMessage(msg Message) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b := s.Buffers[Key(msg.Server, msg.Target)]
	if b == nil {
		return false
	}
	for i := len(b.Messages) - 1; i >= 0; i-- {
		cur := b.Messages[i]
		if msg.RelayID != "" {
			if cur.RelayID == msg.RelayID {
				return true
			}
			continue
		}
		if cur.Time.Equal(msg.Time) && cur.Kind == msg.Kind && cur.Nick == msg.Nick && cur.Text == msg.Text {
			return true
		}
	}
	return false
}

func (s *State) Close(server, target string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := Key(server, target)
	delete(s.Buffers, key)
	for i, k := range s.Order {
		if k == key {
			s.Order = append(s.Order[:i], s.Order[i+1:]...)
			break
		}
	}
	if s.CurrentKey == key {
		if len(s.Order) > 0 {
			s.CurrentKey = s.Order[0]
		} else {
			s.CurrentKey = ""
		}
	}
}

func cloneBuffer(b *Buffer) *Buffer {
	if b == nil {
		return nil
	}
	cp := *b
	cp.Messages = append([]Message(nil), b.Messages...)
	return &cp
}

func FormatMessage(m Message, layout string) string {
	stamp := m.Time.Local().Format(layout)
	switch m.Kind {
	case KindAction:
		return fmt.Sprintf("%s * %s %s", stamp, m.Nick, m.Text)
	case KindNotice:
		return fmt.Sprintf("%s -%s- %s", stamp, m.Nick, m.Text)
	case KindSystem:
		return fmt.Sprintf("%s *** %s", stamp, m.Text)
	case KindError:
		return fmt.Sprintf("%s !!! %s", stamp, m.Text)
	case KindDCC:
		return fmt.Sprintf("%s DCC %s", stamp, m.Text)
	default:
		return fmt.Sprintf("%s <%s> %s", stamp, m.Nick, m.Text)
	}
}

func SortedServers(buffers []*Buffer) []string {
	set := map[string]bool{}
	for _, b := range buffers {
		set[b.Server] = true
	}
	out := make([]string, 0, len(set))
	for server := range set {
		out = append(out, server)
	}
	sort.Strings(out)
	return out
}

func IsChannel(target string) bool {
	return strings.HasPrefix(target, "#") || strings.HasPrefix(target, "&") || strings.HasPrefix(target, "+") || strings.HasPrefix(target, "!")
}

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
	Time    time.Time
	Server  string
	Target  string
	Nick    string
	Text    string
	Kind    Kind
	Tags    map[string]string
	Mention bool
}

type Buffer struct {
	Server   string
	Target   string
	Messages []Message
	Unread   int
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

func Key(server, target string) string { return server + "\x00" + target }

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
	b.Messages = append(b.Messages, msg)
	if len(b.Messages) > s.MaxLines {
		b.Messages = append([]Message(nil), b.Messages[len(b.Messages)-s.MaxLines:]...)
	}
	if s.CurrentKey != Key(msg.Server, msg.Target) {
		b.Unread++
	}
}

func (s *State) Select(server, target string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := s.ensureLocked(server, target)
	s.CurrentKey = Key(server, target)
	b.Unread = 0
}

func (s *State) SelectKey(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.Buffers[key]; !ok {
		return false
	}
	s.CurrentKey = key
	s.Buffers[key].Unread = 0
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

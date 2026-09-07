package model

import (
	"fmt"
	"testing"
	"time"
)

func testMessage(n int) Message {
	return Message{Time: time.Unix(int64(n), 0), Server: "test", Target: "#chan", Text: fmt.Sprintf("m%d", n), Kind: KindMessage}
}

func TestCurrentWindowIncrementalAndTrim(t *testing.T) {
	s := New(3)
	s.Ensure("test", "#chan")
	s.Select("test", "#chan")
	for i := 1; i <= 3; i++ {
		s.Add(testMessage(i))
	}

	w := s.CurrentWindow(0)
	if w == nil || w.Start != 0 || w.Total != 3 || len(w.Messages) != 3 {
		t.Fatalf("initial window = %#v", w)
	}

	// Once history is full, one new message should expose only that delta while
	// advancing Start to tell the UI to discard one cached row.
	s.Add(testMessage(4))
	w = s.CurrentWindow(3)
	if w.Start != 1 || w.Total != 4 || len(w.Messages) != 1 || w.Messages[0].Text != "m4" {
		t.Fatalf("incremental trimmed window = %#v", w)
	}

	// A caller that fell behind the retained window receives the complete
	// retained history so it can rebuild its cache safely.
	s.Add(testMessage(5))
	w = s.CurrentWindow(0)
	if w.Start != 2 || w.Total != 5 || len(w.Messages) != 3 {
		t.Fatalf("full recovery window = %#v", w)
	}
	want := []string{"m3", "m4", "m5"}
	for i, msg := range w.Messages {
		if msg.Text != want[i] {
			t.Fatalf("message %d = %q, want %q", i, msg.Text, want[i])
		}
	}
}

func TestSnapshotInfoDoesNotNeedMessages(t *testing.T) {
	s := New(1000)
	s.Ensure("test", "#chan")
	s.Add(testMessage(1))
	infos, current := s.SnapshotInfo()
	if current != Key("test", "#chan") || len(infos) != 1 {
		t.Fatalf("snapshot info = %#v, current %q", infos, current)
	}
	if infos[0].Server != "test" || infos[0].Target != "#chan" || infos[0].Total != 1 {
		t.Fatalf("snapshot info entry = %#v", infos[0])
	}
}

func TestPrivateQueryTargetsAreCaseInsensitive(t *testing.T) {
	s := New(100)
	s.Select("test", "Leah")

	// The server later supplies the nick's actual/current casing.
	s.Add(Message{
		Time:   time.Unix(1, 0),
		Server: "test",
		Target: "leah",
		Nick:   "leah",
		Text:   "hello",
		Kind:   KindMessage,
	})

	infos, current := s.SnapshotInfo()
	if len(infos) != 1 {
		t.Fatalf("buffer count = %d, want 1: %#v", len(infos), infos)
	}
	if current != Key("test", "leah") || current != Key("test", "Leah") {
		t.Fatalf("current key = %q, want Leah/leah to resolve identically", current)
	}
	if infos[0].Target != "leah" {
		t.Fatalf("display target = %q, want server-provided casing %q", infos[0].Target, "leah")
	}
	if infos[0].Total != 1 {
		t.Fatalf("message total = %d, want 1", infos[0].Total)
	}

	upper := s.Find("test", "Leah")
	lower := s.Find("test", "leah")
	if upper == nil || lower == nil || upper.Total != 1 || lower.Total != 1 {
		t.Fatalf("case-insensitive Find failed: upper=%#v lower=%#v", upper, lower)
	}

	// Selecting either spelling must keep using the one existing query buffer.
	s.Select("test", "LEAH")
	infos, _ = s.SnapshotInfo()
	if len(infos) != 1 {
		t.Fatalf("selecting another case created a duplicate: %#v", infos)
	}
}

func TestContainsMessageDeduplicatesRelayReplay(t *testing.T) {
	s := New(10)
	msg := Message{
		Time:   time.Unix(123, 456),
		Server: "test",
		Target: "#chan",
		Nick:   "alice",
		Text:   "hello",
		Kind:   KindMessage,
	}
	if s.ContainsMessage(msg) {
		t.Fatal("empty state reported message present")
	}
	s.Add(msg)
	if !s.ContainsMessage(msg) {
		t.Fatal("state did not find retained identical message")
	}
	msg.Text = "different"
	if s.ContainsMessage(msg) {
		t.Fatal("state matched different message text")
	}
}

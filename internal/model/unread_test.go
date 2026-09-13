package model

import "testing"

func TestMarkReadDoesNotConsumeConcurrentArrival(t *testing.T) {
	s := New(2)
	s.Add(testMessage(1))
	s.Select("test", "#chan")
	view := s.CurrentInfo()
	s.Add(testMessage(2))
	s.MarkReadThrough(view.Server, view.Target, view.Total)
	if b := s.CurrentInfo(); b.Unread != 1 || b.ReadTotal != 1 {
		t.Fatalf("read state=%#v", b)
	}
	s.Add(testMessage(3))
	s.Add(testMessage(4))
	if b := s.CurrentInfo(); b.Unread != 3 || b.ReadTotal != 1 {
		t.Fatalf("eviction cleared unread: %#v", b)
	}
	s.MarkReadThrough("test", "#chan", 0)
	if s.CurrentInfo().ReadTotal != 1 {
		t.Fatal("read cursor went backwards")
	}
}

func TestSuppressedUnreadRowsRemainVisibleWithoutCountingAsActivity(t *testing.T) {
	s := New(10)
	s.Select("test", "#chan")
	s.Add(Message{Server: "test", Target: "#chan", Kind: KindSystem, Text: "alice joined", SuppressUnread: true})
	if b := s.CurrentInfo(); b.Total != 1 || b.Unread != 0 || b.ReadTotal != 1 {
		t.Fatalf("join row state = %#v, want visible but not unread", b)
	}

	s.Add(testMessage(2))
	s.Add(Message{Server: "test", Target: "#chan", Kind: KindSystem, Text: "alice left", SuppressUnread: true})
	view := s.CurrentInfo()
	if view.Total != 3 || view.Unread != 1 {
		t.Fatalf("part changed activity count: %#v", view)
	}

	// A membership row and a chat message arriving after the rendered snapshot
	// must leave exactly the chat message unread when that snapshot is marked.
	s.Add(Message{Server: "test", Target: "#chan", Kind: KindSystem, Text: "bob quit", SuppressUnread: true})
	s.Add(testMessage(5))
	s.MarkReadThrough(view.Server, view.Target, view.Total)
	if b := s.CurrentInfo(); b.Unread != 1 || b.ReadTotal != view.Total {
		t.Fatalf("concurrent membership/chat state = %#v, want one unread chat", b)
	}
	if b := s.Current(); len(b.Messages) != 5 {
		t.Fatalf("suppressed rows disappeared from transcript: %#v", b)
	}
}

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
	if b := s.CurrentInfo(); b.Unread != 2 || b.ReadTotal != 1 {
		t.Fatalf("retained unread state: %#v", b)
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

func TestUnreadMentionSurvivesEvictionAndClearsWhenRead(t *testing.T) {
	s := New(2)
	s.Select("test", "#active")
	s.Add(Message{Server: "test", Target: "#other", Kind: KindMessage, Mention: true})
	mentionTotal := uint64(1)
	for range 3 {
		s.Add(Message{Server: "test", Target: "#other", Kind: KindMessage})
	}
	infos, _ := s.SnapshotInfo()
	if !infos[1].Mention {
		t.Fatal("evicted unread mention lost its channel highlight")
	}
	s.MarkReadThrough("test", "#other", mentionTotal+1)
	infos, _ = s.SnapshotInfo()
	if infos[1].Mention || infos[1].Unread != 2 {
		t.Fatalf("read mention state = %#v", infos[1])
	}
	s.Add(Message{Server: "test", Target: "#other", Kind: KindAction, Mention: true})
	s.MarkReadThrough("test", "#other", 4)
	infos, _ = s.SnapshotInfo()
	if !infos[1].Mention {
		t.Fatal("reading an older snapshot cleared a newer mention")
	}
	s.Select("test", "#other")
	if s.CurrentInfo().Mention {
		t.Fatal("selecting channel did not clear mention")
	}
	s.Add(Message{Server: "test", Target: "#other", Kind: KindMessage, Mention: true})
	s.SelectKey(Key("test", "#other"))
	if s.CurrentInfo().Mention {
		t.Fatal("selecting channel by key did not clear mention")
	}
}

func TestUnreadCountsOnlyRetainedActivity(t *testing.T) {
	s := New(3)
	s.Select("test", "#active")
	for i := 0; i < 20; i++ {
		msg := testMessage(i)
		msg.Replay = true
		msg.SuppressUnread = i%2 == 0
		s.Add(msg)
		b := s.Find("test", "#chan")
		want := 0
		for _, retained := range b.Messages {
			if !retained.SuppressUnread {
				want++
			}
		}
		if b.Unread != want {
			t.Fatalf("after replay %d: unread=%d, retained activity=%d", i, b.Unread, want)
		}
	}
	s.Select("test", "#chan")
	for i := 20; i < 25; i++ {
		s.Add(testMessage(i))
	}
	if b := s.CurrentInfo(); b.Unread != 3 {
		t.Fatalf("after read and eviction: %#v", b)
	}
}

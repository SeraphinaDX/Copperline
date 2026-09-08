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

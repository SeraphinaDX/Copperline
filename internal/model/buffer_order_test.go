package model

import (
	"reflect"
	"testing"
)

func TestSortBuffersAlphabeticallyPreservesStateAndNetworkOrder(t *testing.T) {
	s := New(100)
	created := []BufferInfo{
		{Server: "z-network", Target: "#zebra"},
		{Server: "a-network", Target: "#last"},
		{Server: "z-network", Target: "#Beta"},
		{Server: "z-network", Target: "*server*"},
		{Server: "a-network", Target: "*server*"},
		{Server: "z-network", Target: "#alpha"},
		{Server: "z-network", Target: "Zoe"},
		{Server: "z-network", Target: "alice"},
	}
	for _, b := range created {
		s.Ensure(b.Server, b.Target)
	}
	s.Select("z-network", "#zebra")
	before, selected := s.SnapshotInfo()
	ordered, _ := s.SnapshotInfo()
	SortBuffersAlphabetically(ordered)
	want := []BufferInfo{created[3], created[5], created[2], created[0], created[7], created[6], created[4], created[1]}
	if !reflect.DeepEqual(ordered, want) {
		t.Fatalf("order = %#v, want %#v", ordered, want)
	}
	after, current := s.SnapshotInfo()
	if !reflect.DeepEqual(after, before) || current != selected {
		t.Fatal("presentation sorting changed creation order or selected buffer")
	}
}

func TestSortBuffersAlphabeticallyStableTiesAndEmpty(t *testing.T) {
	SortBuffersAlphabetically(nil)
	buffers := []BufferInfo{{Server: "test", Target: "Alice", Unread: 2}, {Server: "test", Target: "alice", Unread: 1}}
	want := append([]BufferInfo(nil), buffers...)
	SortBuffersAlphabetically(buffers)
	if !reflect.DeepEqual(buffers, want) {
		t.Fatal("case-insensitive ties changed order or unread metadata")
	}
}

package urlgrab

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"copperline/internal/model"
)

func TestExtract(t *testing.T) {
	text := "See (https://example.org/wiki/Go_(language)), <https://example.org/?a=1&b=2>. Again https://example.org/?a=1&b=2 and HTTPS://example.org/end. Not https:// or ftp://example.org"
	want := []string{"https://example.org/wiki/Go_(language)", "https://example.org/?a=1&b=2", "HTTPS://example.org/end"}
	if got := Extract(text); !reflect.DeepEqual(got, want) {
		t.Fatalf("links = %q, want %q", got, want)
	}
}

func TestCollectorAppendAndReplay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "links", "urls.jsonl")
	msg := model.Message{Time: time.Unix(123, 0), Server: "net", Target: "#chan", Nick: "alice", Kind: model.KindMessage, Text: "\x0302https://example.org\x0f"}
	for i := 0; i < 2; i++ {
		c := New(path, func(err error) { t.Error(err) })
		c.Add(msg)
		duplicate := msg
		duplicate.Target = "#other"
		duplicate.Text = "HTTPS://EXAMPLE.ORG"
		c.Add(duplicate)
		replay := msg
		replay.Replay = true
		c.Add(replay)
		c.Close()
		c.Close()
		c.Add(msg) // shutdown racing a final callback is harmless
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	count := 0
	for s.Scan() {
		var record Record
		if err := json.Unmarshal(s.Bytes(), &record); err != nil {
			t.Fatal(err)
		}
		if record.URL != "https://example.org" || record.Target != "#chan" || record.Nick != "alice" || !record.Time.Equal(msg.Time) {
			t.Fatalf("bad record: %+v", record)
		}
		count++
	}
	if s.Err() != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, s.Err())
	}
}

func TestCollectorReportsWriteFailure(t *testing.T) {
	path := t.TempDir() // a directory cannot be opened as a log file
	errors := make(chan error, 2)
	c := New(path, func(err error) { errors <- err })
	for i := 0; i < 2; i++ {
		c.Add(model.Message{Kind: model.KindMessage, Text: "https://example.org"})
	}
	c.Close()
	select {
	case <-errors:
	case <-time.After(time.Second):
		t.Fatal("write failure not reported")
	}
	if len(errors) != 0 {
		t.Fatal("repeated write failure flooded reports")
	}
}

func TestFullQueueDoesNotBlockDelivery(t *testing.T) {
	c := &Collector{queue: make(chan []Record, 1), report: func(error) {}}
	msg := model.Message{Kind: model.KindMessage, Text: "https://example.org"}
	c.Add(msg)
	done := make(chan struct{})
	go func() { c.Add(msg); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("full URL queue blocked IRC delivery")
	}
}

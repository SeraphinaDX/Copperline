package tui

import (
	"testing"

	"github.com/metaspartan/gotui/v5/widgets"
)

func TestScrollTranscriptBottomEmptyListIsSafe(t *testing.T) {
	list := &transcriptList{List: widgets.NewList()}
	app := &App{transcript: list}

	app.scrollTranscriptBottom()

	if list.SelectedRow != 0 {
		t.Fatalf("SelectedRow = %d, want 0 for empty transcript", list.SelectedRow)
	}
}

func TestScrollTranscriptBottomSelectsLastRow(t *testing.T) {
	list := &transcriptList{List: widgets.NewList()}
	list.Rows = []string{"one", "two", "three"}
	app := &App{transcript: list}

	app.scrollTranscriptBottom()

	if list.SelectedRow != 2 {
		t.Fatalf("SelectedRow = %d, want 2", list.SelectedRow)
	}
}

func TestTranscriptCachePreservesBacklogAndLiveRows(t *testing.T) {
	list := &transcriptList{List: widgets.NewList()}
	list.Rows = []string{
		"[old persisted line](fg:#77839a)",
		"live message one",
		"live message two",
	}
	app := &App{
		transcript:       list,
		transcriptCaches: make(map[string]transcriptCache),
	}

	key := "libera\x00#copperline"
	app.saveTranscriptCache(key, transcriptCache{
		rows:        list.Rows,
		start:       12,
		total:       14,
		fromLog:     true,
		backlogRows: 1,
	})

	// Mutating the current widget after saving must not mutate the cache.
	app.transcript.Rows[1] = "changed elsewhere"
	app.transcript.Rows = nil
	app.transcriptStart = 0
	app.transcriptTotal = 0
	app.transcriptFromLog = false
	app.transcriptBacklogRows = 0

	if !app.restoreTranscriptCache(key) {
		t.Fatal("restoreTranscriptCache returned false")
	}
	want := []string{
		"[old persisted line](fg:#77839a)",
		"live message one",
		"live message two",
	}
	if len(app.transcript.Rows) != len(want) {
		t.Fatalf("restored %d rows, want %d", len(app.transcript.Rows), len(want))
	}
	for i := range want {
		if app.transcript.Rows[i] != want[i] {
			t.Fatalf("row %d = %q, want %q", i, app.transcript.Rows[i], want[i])
		}
	}
	if app.transcriptStart != 12 || app.transcriptTotal != 14 {
		t.Fatalf("restored window = start %d total %d, want 12/14", app.transcriptStart, app.transcriptTotal)
	}
	if !app.transcriptFromLog || app.transcriptBacklogRows != 1 {
		t.Fatalf("restored backlog metadata = fromLog %v rows %d, want true/1", app.transcriptFromLog, app.transcriptBacklogRows)
	}
}

package tui

import (
	"testing"

	"github.com/metaspartan/gotui/v5/widgets"
)

func TestScrollTranscriptBottomEmptyListIsSafe(t *testing.T) {
	list := widgets.NewList()
	app := &App{transcript: list}

	app.scrollTranscriptBottom()

	if list.SelectedRow != 0 {
		t.Fatalf("SelectedRow = %d, want 0 for empty transcript", list.SelectedRow)
	}
}

func TestScrollTranscriptBottomSelectsLastRow(t *testing.T) {
	list := widgets.NewList()
	list.Rows = []string{"one", "two", "three"}
	app := &App{transcript: list}

	app.scrollTranscriptBottom()

	if list.SelectedRow != 2 {
		t.Fatalf("SelectedRow = %d, want 2", list.SelectedRow)
	}
}

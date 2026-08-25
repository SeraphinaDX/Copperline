package tui

import (
	"testing"

	"copperline/internal/config"

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

func TestKeyBindingMatchesRuneFallbackForModernCtrlEvents(t *testing.T) {
	if !keyBindingMatchesRuneFallback("Ctrl+Y", "y", true, false) {
		t.Fatal("Ctrl+Y did not match KeyRune y carrying Ctrl modifier")
	}
	if !keyBindingMatchesRuneFallback("Ctrl+Y", string(rune(25)), true, false) {
		t.Fatal("Ctrl+Y did not match ASCII Ctrl+Y control byte fallback")
	}
	if keyBindingMatchesRuneFallback("Alt+Y", "y", true, false) {
		t.Fatal("Ctrl rune incorrectly matched Alt+Y")
	}
	if !keyBindingMatchesRuneFallback("Alt+N", "n", false, true) {
		t.Fatal("Alt+N did not match KeyRune n carrying Alt modifier")
	}
}

func TestUserListScrollKeyDeltaConfigurable(t *testing.T) {
	keys := config.KeybindingsConfig{UserListDown: "Alt+N", UserListUp: "Alt+P"}
	tests := []struct {
		name string
		id   string
		want int
	}{
		{name: "alt n meta form", id: "<M-n>", want: 1},
		{name: "alt p meta form", id: "<M-p>", want: -1},
		{name: "alt n alternate form", id: "<A-n>", want: 1},
		{name: "alt p long form", id: "<Alt-p>", want: -1},
		{name: "plain ctrl n remains buffer navigation", id: "<C-n>", want: 0},
		{name: "plain ctrl p remains buffer navigation", id: "<C-p>", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := userListScrollKeyDelta(tt.id, keys)
			if got != tt.want {
				t.Fatalf("userListScrollKeyDelta(%q) = %d, want %d", tt.id, got, tt.want)
			}
		})
	}

	keys.UserListDown = "Ctrl+J"
	if got := userListScrollKeyDelta("<C-j>", keys); got != 1 {
		t.Fatalf("custom Ctrl+J user-list binding = %d, want 1", got)
	}
	if got := userListScrollKeyDelta("<M-n>", keys); got != 0 {
		t.Fatalf("old Alt+N binding still active after rebind: %d", got)
	}
}

func TestClampUserScroll(t *testing.T) {
	tests := []struct {
		scroll  int
		total   int
		visible int
		want    int
	}{
		{scroll: -3, total: 50, visible: 10, want: 0},
		{scroll: 5, total: 50, visible: 10, want: 5},
		{scroll: 99, total: 50, visible: 10, want: 40},
		{scroll: 7, total: 5, visible: 10, want: 0},
		{scroll: 7, total: 50, visible: 0, want: 0},
	}
	for _, tt := range tests {
		if got := clampUserScroll(tt.scroll, tt.total, tt.visible); got != tt.want {
			t.Fatalf("clampUserScroll(%d, %d, %d) = %d, want %d", tt.scroll, tt.total, tt.visible, got, tt.want)
		}
	}
}

func TestUserNickAtVisibleRowUsesScrollOffset(t *testing.T) {
	users := widgets.NewList()
	users.Rows = []string{"Carol", "Dave"}
	app := &App{
		users:      users,
		userNicks:  []string{"Alice", "Bob", "Carol", "Dave"},
		userScroll: 2,
	}

	nick, ok := app.userNickAtVisibleRow(1)
	if !ok {
		t.Fatal("userNickAtVisibleRow returned !ok")
	}
	if nick != "Dave" {
		t.Fatalf("nick = %q, want Dave", nick)
	}
}

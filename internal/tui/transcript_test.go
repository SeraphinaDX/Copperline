package tui

import (
	"strings"
	"testing"
	"time"

	"copperline/internal/config"
	"copperline/internal/model"

	ui "github.com/metaspartan/gotui/v5"
	"github.com/metaspartan/gotui/v5/widgets"
)

func TestTranscriptViewportIncludesNewestAfterWrappedRow(t *testing.T) {
	list := &transcriptList{List: widgets.NewList()}
	list.WrapText = true
	// With borders this gives an inner viewport of 12 columns x 3 rows.
	list.SetRect(0, 0, 14, 5)
	list.Rows = []string{
		"old",
		"this message wraps onto more than one terminal line",
		"previous",
		"newest",
	}
	list.SelectedRow = len(list.Rows) - 1

	lines, _, _ := list.visiblePhysicalLines()
	if len(lines) == 0 {
		t.Fatal("visiblePhysicalLines returned no lines")
	}
	if got := lines[len(lines)-1].logicalRow; got != len(list.Rows)-1 {
		t.Fatalf("last visible logical row = %d, want newest row %d", got, len(list.Rows)-1)
	}
	if len(lines) > list.Inner.Dy() {
		t.Fatalf("rendered %d physical lines into height %d", len(lines), list.Inner.Dy())
	}
}

func TestTranscriptScrollBottomNeverLagsOneLogicalRow(t *testing.T) {
	list := &transcriptList{List: widgets.NewList()}
	list.WrapText = true
	list.SetRect(0, 0, 18, 6)
	list.Rows = []string{
		"one",
		"two wraps because this line is deliberately long",
		"three",
		"four",
	}

	for i := 0; i < 20; i++ {
		list.Rows = append(list.Rows, "new message")
		if len(list.Rows) > 10 {
			list.Rows = list.Rows[1:]
		}
		list.ScrollBottom()
		lines, _, _ := list.visiblePhysicalLines()
		if len(lines) == 0 {
			t.Fatalf("iteration %d: no visible lines", i)
		}
		if got := lines[len(lines)-1].logicalRow; got != len(list.Rows)-1 {
			t.Fatalf("iteration %d: last visible logical row = %d, want %d", i, got, len(list.Rows)-1)
		}
	}
}

func TestTranscriptMentionWithBracketNickDoesNotCorruptMarkup(t *testing.T) {
	theme := uiTheme{cfg: config.ThemeConfig{
		Timestamp: "#77839a",
		Mention:   "#ff6f91",
	}}
	msg := model.Message{
		Time:    time.Unix(0, 0),
		Nick:    "[",
		Text:    "britney: https://en.wikipedia.org/wiki/Legal_fiction",
		Kind:    model.KindMessage,
		Mention: true,
	}

	row := theme.formatMessage(msg, "X")
	got := ui.CellsToString(parseTranscriptStyles(row, ui.NewStyle(ui.ColorWhite)))
	want := "X ! <[> britney: https://en.wikipedia.org/wiki/Legal_fiction"
	if got != want {
		t.Fatalf("rendered mention = %q, want %q", got, want)
	}
	if strings.Contains(got, "](fg:") || strings.Contains(got, "(fg:") {
		t.Fatalf("renderer leaked style markup into visible text: %q", got)
	}
}

func TestTranscriptDoesNotInterpretIRCTextAsGotuiMarkup(t *testing.T) {
	theme := uiTheme{cfg: config.ThemeConfig{
		Timestamp:  "#77839a",
		Foreground: "#ffffff",
		NickColors: []string{"#64d8cb"},
	}}
	msg := model.Message{
		Time: time.Unix(0, 0),
		Nick: "someone",
		Text: "literal [not markup](fg:#ff0000) stays literal",
		Kind: model.KindMessage,
	}

	row := theme.formatMessage(msg, "X")
	got := ui.CellsToString(parseTranscriptStyles(row, ui.NewStyle(ui.ColorWhite)))
	want := "X <someone> literal [not markup](fg:#ff0000) stays literal"
	if got != want {
		t.Fatalf("rendered message = %q, want %q", got, want)
	}
}

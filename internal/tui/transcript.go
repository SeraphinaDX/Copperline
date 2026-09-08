package tui

import (
	"image"
	"strings"

	ui "github.com/metaspartan/gotui/v5"
	"github.com/metaspartan/gotui/v5/widgets"
)

// transcriptList is a thin wrapper around gotui's List used only for the
// conversation transcript. gotui v5.0.3 scrolls List in logical Rows, but
// WrapText renders one logical row as one or more physical terminal lines.
// Once a wrapped row is inside the viewport, List.ScrollBottom can therefore
// leave the newest logical row below the visible screen. The next IRC message
// shifts the previously hidden row upward, which looks exactly like chat is
// one message behind.
//
// Keep the public List API that the rest of Copperline already uses, but draw
// the visible window from the selected row backwards in physical wrapped
// lines. With live-follow SelectedRow is the newest message, so its final
// wrapped line is always visible at the bottom of the transcript.
type transcriptList struct {
	*widgets.List
	unread       bool
	unreadBefore int
	searchQuery  string
	searchMinRow int
}

type transcriptPhysicalLine struct {
	logicalRow int
	cells      []ui.Cell
}

func parseTranscriptStyles(row string, defaultStyle ui.Style) []ui.Cell {
	cells := make([]ui.Cell, 0, len(row))

	appendText := func(text string, style ui.Style) {
		if text == "" {
			return
		}
		cells = append(cells, ui.RunesToStyledCells([]rune(text), style)...)
	}

	for len(row) > 0 {
		start := strings.Index(row, transcriptStyleStart)
		if start < 0 {
			appendText(row, defaultStyle)
			break
		}

		appendText(row[:start], defaultStyle)

		rest := row[start+len(transcriptStyleStart):]
		colorEnd := strings.IndexByte(rest, 0)
		if colorEnd < 0 {
			// A malformed internal marker must never eat visible text.
			appendText(row[start:], defaultStyle)
			break
		}

		color := rest[:colorEnd]
		styledText := rest[colorEnd+1:]
		end := strings.Index(styledText, transcriptStyleEnd)
		if end < 0 {
			appendText(row[start:], defaultStyle)
			break
		}

		style := defaultStyle
		style.Fg = colorSpec(color, defaultStyle.Fg)
		appendText(styledText[:end], style)
		row = styledText[end+len(transcriptStyleEnd):]
	}

	return cells
}

func (t *transcriptList) Draw(buf *ui.Buffer) {
	t.List.Block.Draw(buf)

	if len(t.Rows) == 0 || t.Inner.Dx() <= 0 || t.Inner.Dy() <= 0 {
		return
	}

	if t.SelectedRow < 0 {
		t.SelectedRow = 0
	}
	if t.SelectedRow >= len(t.Rows) {
		t.SelectedRow = len(t.Rows) - 1
	}

	lines, hasOlder, hasNewer := t.visiblePhysicalLines()
	y := t.Inner.Min.Y
	for _, line := range lines {
		if y >= t.Inner.Max.Y {
			break
		}
		t.drawPhysicalLine(buf, line, y)
		y++
	}

	// Match gotui List's scroll hints, but base them on the physical viewport.
	if hasOlder && t.Inner.Dx() > 0 {
		buf.SetCell(
			ui.NewCell(ui.UP_ARROW, ui.NewStyle(ui.ColorWhite)),
			image.Pt(t.Inner.Max.X-1, t.Inner.Min.Y),
		)
	}
	if hasNewer && t.Inner.Dx() > 0 {
		buf.SetCell(
			ui.NewCell(ui.DOWN_ARROW, ui.NewStyle(ui.ColorWhite)),
			image.Pt(t.Inner.Max.X-1, t.Inner.Max.Y-1),
		)
	}
}

// visiblePhysicalLines returns at most one terminal screen of wrapped lines,
// ending at SelectedRow. This intentionally does not walk the entire history:
// it stops as soon as the viewport is full.
func (t *transcriptList) visiblePhysicalLines() ([]transcriptPhysicalLine, bool, bool) {
	height := t.Inner.Dy()
	width := t.Inner.Dx()
	if height <= 0 || width <= 0 || len(t.Rows) == 0 {
		return nil, false, false
	}

	selected := t.SelectedRow
	if selected < 0 {
		selected = 0
	}
	if selected >= len(t.Rows) {
		selected = len(t.Rows) - 1
	}

	// Build backwards so the selected row's final physical line is guaranteed
	// to be visible. Reverse once at the end for normal top-to-bottom drawing.
	reversed := make([]transcriptPhysicalLine, 0, height)
	hasOlder := false

	for row := selected; row >= 0 && len(reversed) < height; row-- {
		wrapped := t.wrapLogicalRow(row, width)
		for i := len(wrapped) - 1; i >= 0 && len(reversed) < height; i-- {
			reversed = append(reversed, transcriptPhysicalLine{logicalRow: row, cells: wrapped[i]})
			if len(reversed) == height {
				// There is older visible content either earlier in this same
				// wrapped logical row or in an older logical row.
				hasOlder = i > 0 || row > 0
				break
			}
		}
	}

	lines := make([]transcriptPhysicalLine, len(reversed))
	for i := range reversed {
		lines[len(reversed)-1-i] = reversed[i]
	}

	hasNewer := selected < len(t.Rows)-1
	return lines, hasOlder, hasNewer
}

func (t *transcriptList) wrapLogicalRow(row, width int) [][]ui.Cell {
	cells := parseTranscriptStyles(t.Rows[row], t.TextStyle)
	if t.searchQuery != "" && row >= t.searchMinRow {
		text := make([]rune, len(cells))
		for i, cell := range cells {
			text[i] = cell.Rune
		}
		for _, span := range searchRanges(text, t.searchQuery) {
			for i := span[0]; i < span[1]; i++ {
				cells[i].Style.Bg = ui.ColorYellow
				cells[i].Style.Fg = ui.ColorBlack
			}
		}
	}

	// Preserve List's selected-row styling semantics. Copperline normally sets
	// SelectedStyle == TextStyle for transcripts, but keeping this behavior here
	// makes the wrapper faithful if that ever changes.
	if row == t.SelectedRow {
		for i := range cells {
			if cells[i].Style.Fg == t.TextStyle.Fg && cells[i].Style.Bg == t.TextStyle.Bg {
				cells[i].Style = t.SelectedStyle
			}
		}
	}

	if t.WrapText && width > 0 {
		cells = ui.WrapCells(cells, uint(width))
	}
	lines := ui.SplitCells(cells, '\n')
	if t.unread && row == t.unreadBefore {
		divider := ui.RunesToStyledCells([]rune("── New messages below ──"), ui.NewStyle(ui.ColorYellow))
		lines = append([][]ui.Cell{ui.TrimCells(divider, width)}, lines...)
	}
	if len(lines) == 0 {
		// An empty logical row still occupies one terminal line.
		return [][]ui.Cell{{}}
	}
	return lines
}

func (t *transcriptList) drawPhysicalLine(buf *ui.Buffer, line transcriptPhysicalLine, y int) {
	cells := ui.TrimCells(line.cells, t.Inner.Dx())
	positioned := ui.BuildCellWithXArray(cells)
	if len(positioned) == 0 {
		return
	}

	xOffset := 0
	last := positioned[len(positioned)-1]
	rowWidth := last.X + 1
	switch t.TextAlignment {
	case ui.AlignCenter:
		xOffset = (t.Inner.Dx() - rowWidth) / 2
	case ui.AlignRight:
		xOffset = t.Inner.Dx() - rowWidth
	}
	if xOffset < 0 {
		xOffset = 0
	}

	for _, cx := range positioned {
		x := t.Inner.Min.X + xOffset + cx.X
		if x >= t.Inner.Max.X {
			break
		}
		buf.SetCell(cx.Cell, image.Pt(x, y))
	}
}

func (t *transcriptList) ScrollUp() {
	t.ScrollAmount(-1)
}

func (t *transcriptList) ScrollDown() {
	t.ScrollAmount(1)
}

func (t *transcriptList) ScrollAmount(amount int) {
	if len(t.Rows) == 0 {
		t.SelectedRow = 0
		return
	}
	selected := t.SelectedRow + amount
	if selected < 0 {
		selected = 0
	}
	if selected >= len(t.Rows) {
		selected = len(t.Rows) - 1
	}
	t.SelectedRow = selected
}

func (t *transcriptList) ScrollPageUp() {
	amount := t.Inner.Dy()
	if amount < 1 {
		amount = 1
	}
	t.ScrollAmount(-amount)
}

func (t *transcriptList) ScrollPageDown() {
	amount := t.Inner.Dy()
	if amount < 1 {
		amount = 1
	}
	t.ScrollAmount(amount)
}

func (t *transcriptList) ScrollTop() {
	t.SelectedRow = 0
}

func (t *transcriptList) ScrollBottom() {
	if len(t.Rows) == 0 {
		t.SelectedRow = 0
		return
	}
	t.SelectedRow = len(t.Rows) - 1
}

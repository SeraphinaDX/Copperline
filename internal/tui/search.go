package tui

import (
	"fmt"
	"strings"
	"unicode"
)

type searchState struct {
	query    string
	label    string
	sequence uint64
	found    bool
}

func (a *App) startSearch(query string) {
	// Commit any pending buffer switch before installing its search state.
	a.rebuildCurrent()
	a.search = searchState{query: strings.TrimSpace(query)}
	if a.search.query != "" {
		a.moveSearch(1)
	}
}

func (a *App) moveSearch(direction int) {
	if a.search.query == "" {
		return
	}
	a.rebuildCurrent()
	if a.search.query == "" {
		return
	} // a buffer change cancels the old search
	w := a.state.CurrentWindow(0)
	if w == nil {
		return
	}
	var matches []uint64
	for i, msg := range w.Messages {
		sequence := w.Start + uint64(i)
		// Network delivery can advance state after rebuildCurrent took its
		// snapshot. Search only rows actually present in that rendered snapshot.
		if sequence >= a.transcriptTotal {
			break
		}
		if len(searchRanges([]rune(msg.Nick+" "+msg.Text), a.search.query)) > 0 {
			matches = append(matches, sequence)
		}
	}
	if len(matches) == 0 {
		a.search.found = false
		a.search.label = "search: no matches"
		return
	}
	index := 0
	if direction < 0 {
		index = len(matches) - 1
	}
	if a.search.found {
		if direction > 0 {
			for i, n := range matches {
				if n > a.search.sequence {
					index = i
					break
				}
			}
		} else {
			for i := len(matches) - 1; i >= 0; i-- {
				if matches[i] < a.search.sequence {
					index = i
					break
				}
			}
		}
	}
	a.search.sequence = matches[index]
	a.search.found = true
	a.search.label = fmt.Sprintf("search %d/%d", index+1, len(matches))
	a.follow = false
	// Each retained message occupies one logical row, including wrapped text.
	row := len(a.transcript.Rows) - int(a.transcriptTotal-a.search.sequence)
	if row < 0 {
		row = 0
	}
	a.transcript.SelectedRow = row
}

// Rune-based offsets preserve non-ASCII text even when case folding changes
// its UTF-8 byte length. Matches are literal, case-insensitive, non-overlapping.
func searchRanges(text []rune, query string) [][2]int {
	needle := []rune(query)
	if len(needle) == 0 {
		return nil
	}
	var out [][2]int
	for i := 0; i+len(needle) <= len(text); i++ {
		match := true
		for j, r := range needle {
			if unicode.ToLower(text[i+j]) != unicode.ToLower(r) {
				match = false
				break
			}
		}
		if match {
			out = append(out, [2]int{i, i + len(needle)})
			i += len(needle) - 1
		}
	}
	return out
}

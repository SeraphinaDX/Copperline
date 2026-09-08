package tui

import (
	"fmt"

	"copperline/internal/model"
)

func (a *App) selectNextUnread() {
	keys, current := a.sidebarOrder()
	start := -1
	for i, key := range keys {
		if key == current {
			start = i
			break
		}
	}
	infos, _ := a.state.SnapshotInfo()
	unread := make(map[string]bool, len(infos))
	for _, b := range infos {
		unread[model.Key(b.Server, b.Target)] = b.Unread > 0
	}
	for step := 1; step <= len(keys); step++ {
		key := keys[(start+step)%len(keys)]
		if key != current && unread[key] {
			a.state.SelectKey(key)
			a.follow = false
			a.resetNickCompletion()
			return
		}
	}
}

func (a *App) updateCatchup(b model.BufferInfo, bufferChanged bool) {
	a.transcript.unread = !a.follow && b.Unread > 0
	if a.transcript.unread {
		first := b.ReadTotal
		if first < a.transcriptStart {
			first = a.transcriptStart
		}
		row := len(a.transcript.Rows) - int(b.Total-first)
		if row < 0 {
			row = 0
		}
		a.transcript.unreadBefore = row
		if bufferChanged {
			a.transcript.SelectedRow = row
		}
		a.transcript.Title += fmt.Sprintf(" | %d unread", b.Unread)
	}
	a.transcript.searchQuery = a.search.query
	a.transcript.searchMinRow = len(a.transcript.Rows) - int(b.Total-a.transcriptStart)
	if a.search.query != "" {
		a.transcript.Title += " | " + a.search.label
	}
}

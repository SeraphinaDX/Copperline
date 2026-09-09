package tui

import (
	"time"

	ui "github.com/metaspartan/gotui/v5"
)

func (a *App) handleMouse(e ui.Event) {
	m, ok := mousePayload(e.Payload)
	if !ok {
		return
	}
	w, _ := ui.TerminalDimensions()
	left, _ := a.panelWidths(w)

	switch e.ID {
	case "<MouseWheelUp>":
		if a.mouseOverUsers(m.X, m.Y) {
			a.scrollUsers(-3)
			return
		}
		if m.X >= left {
			a.follow = false
			a.transcript.ScrollAmount(-3)
		}
	case "<MouseWheelDown>":
		if a.mouseOverUsers(m.X, m.Y) {
			a.scrollUsers(3)
			return
		}
		if m.X >= left {
			a.transcript.ScrollAmount(3)
			a.resumeFollowAtBottom()
		}
	case "<MouseLeft>":
		if a.mouseOverRelayReconnect(m.X, m.Y) {
			a.requestRelayReconnect()
			return
		}
		if m.X < left && m.Y > 0 {
			a.clearNickClick()
			row := m.Y - 1
			if row >= 0 && row < len(a.sidebarKeys) {
				if a.state.SelectKey(a.sidebarKeys[row]) {
					a.follow = true
				}
			}
			return
		}
		if len(a.userNicks) > 0 && a.mouseOverUserRows(m.X, m.Y) {
			// The Users widget has a one-cell border, so row zero starts at Y=1.
			visibleRow := m.Y - 1
			if nick, ok := a.userNickAtVisibleRow(visibleRow); ok {
				if b := a.state.CurrentInfo(); b != nil {
					if a.nickDoubleClicked(b.Server, nick) {
						a.state.Select(b.Server, nick)
						a.follow = true
					}
					return
				}
			}
		}
		a.clearNickClick()
	}
}

func (a *App) mouseOverRelayReconnect(x, y int) bool {
	buttonWidth := a.relayReconnectControlWidth()
	if buttonWidth == 0 {
		return false
	}
	w, h := ui.TerminalDimensions()
	if y != h-1 || x < 0 || x >= w {
		return false
	}
	// The relay action owns a dedicated bottom-right region, independent of the
	// ordinary status text, so temporary status modes cannot move its hit box.
	return x >= w-buttonWidth
}

const (
	userPaneWidth = 22
	uiBottomRows  = 4
)

// userListVisibleRows is the number of nickname rows inside the Users border.
func userListVisibleRows(height int) int {
	rows := height - uiBottomRows - 2
	if rows < 0 {
		return 0
	}
	return rows
}

func userListVisibleRowsForCurrentTerminal() int {
	_, h := ui.TerminalDimensions()
	return userListVisibleRows(h)
}

func clampUserScroll(scroll, total, visible int) int {
	if visible <= 0 || total <= visible {
		return 0
	}
	maxScroll := total - visible
	if scroll < 0 {
		return 0
	}
	if scroll > maxScroll {
		return maxScroll
	}
	return scroll
}

func (a *App) scrollUsers(amount int) {
	w, h := ui.TerminalDimensions()
	_, right := a.panelWidths(w)
	if right == 0 || amount == 0 {
		return
	}
	visible := userListVisibleRows(h)
	a.userScroll = clampUserScroll(a.userScroll+amount, len(a.userNicks), visible)
	a.refreshUserRows(visible)
}

func (a *App) refreshUserRows(visible int) {
	a.userScroll = clampUserScroll(a.userScroll, len(a.userNicks), visible)
	if visible <= 0 || len(a.userNicks) == 0 {
		a.users.Rows = nil
		a.users.Title = "Users"
		return
	}

	end := a.userScroll + visible
	if end > len(a.userNicks) {
		end = len(a.userNicks)
	}
	a.users.Rows = make([]string, 0, end-a.userScroll)
	b := a.state.CurrentInfo()
	for _, nick := range a.userNicks[a.userScroll:end] {
		displayNick := nick
		if b != nil {
			displayNick = a.irc.NickPrefix(b.Server, b.Target, nick) + nick
		}
		a.users.Rows = append(a.users.Rows, styled(displayNick, a.theme.nickColor(nick)))
	}

	title := "Users"
	if a.userScroll > 0 {
		title += " ↑"
	}
	if end < len(a.userNicks) {
		title += " ↓"
	}
	a.users.Title = title
	// Rows is already the visible window, so keep gotui's own private scroll
	// offset pinned to the first row.
	a.users.SelectedRow = 0
}

func (a *App) mouseOverUsers(x, y int) bool {
	w, h := ui.TerminalDimensions()
	_, right := a.panelWidths(w)
	if right == 0 {
		return false
	}
	return x >= w-userPaneWidth && x < w && y >= 0 && y < h-uiBottomRows
}

func (a *App) mouseOverUserRows(x, y int) bool {
	w, h := ui.TerminalDimensions()
	_, right := a.panelWidths(w)
	if right == 0 {
		return false
	}
	return x > w-userPaneWidth && x < w-1 && y > 0 && y < h-uiBottomRows-1
}

func (a *App) userNickAtVisibleRow(row int) (string, bool) {
	if row < 0 || a.users == nil || row >= len(a.users.Rows) {
		return "", false
	}
	index := a.userScroll + row
	if index < 0 || index >= len(a.userNicks) {
		return "", false
	}
	return a.userNicks[index], true
}

const nickDoubleClickWindow = 500 * time.Millisecond

// nickDoubleClicked records a click on a nick and reports whether it completes
// a double click. Both clicks must target the same nick on the same server and
// occur close together; otherwise the newest click becomes the first click of
// a new pair.
func (a *App) nickDoubleClicked(server, nick string) bool {
	now := time.Now()
	if a.lastNickClickServer == server &&
		a.lastNickClickNick == nick &&
		!a.lastNickClickAt.IsZero() &&
		now.Sub(a.lastNickClickAt) <= nickDoubleClickWindow {
		a.clearNickClick()
		return true
	}

	a.lastNickClickServer = server
	a.lastNickClickNick = nick
	a.lastNickClickAt = now
	return false
}

func (a *App) clearNickClick() {
	a.lastNickClickServer = ""
	a.lastNickClickNick = ""
	a.lastNickClickAt = time.Time{}
}

// resumeFollowAtBottom restores live-follow once manual downward scrolling
// reaches the newest logical row. Scrolling upward deliberately disables
// follow mode, but arriving back at the bottom should make future messages
// stay visible without requiring End.
func (a *App) resumeFollowAtBottom() {
	if len(a.transcript.Rows) == 0 {
		a.follow = true
		return
	}
	if a.transcript.SelectedRow >= len(a.transcript.Rows)-1 {
		a.follow = true
		a.scrollTranscriptBottom()
	}
}

// scrollTranscriptBottom is the only safe way Copperline should ask gotui's
// List to follow the bottom. gotui v5.0.3 sets SelectedRow to -1 when Rows is
// empty; on the next Draw that can make its private topRow negative and panic.
func (a *App) scrollTranscriptBottom() {
	if a.transcript == nil || len(a.transcript.Rows) == 0 {
		if a.transcript != nil {
			a.transcript.SelectedRow = 0
		}
		return
	}
	a.transcript.ScrollBottom()
}

func mousePayload(v any) (ui.Mouse, bool) {
	switch m := v.(type) {
	case ui.Mouse:
		return m, true
	case *ui.Mouse:
		if m != nil {
			return *m, true
		}
	}
	return ui.Mouse{}, false
}

// completeNick completes the nickname fragment immediately before the input
// cursor. At the start of a message it uses the conventional IRC reply form
// "Nick: ". Repeated Tab presses cycle through all matching users.

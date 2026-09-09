package tui

import (
	"fmt"
	"strconv"
	"time"

	"copperline/internal/model"

	ui "github.com/metaspartan/gotui/v5"
)

func (a *App) render() {
	w, h := ui.TerminalDimensions()
	if w < 40 || h < 10 {
		return
	}
	left, right := a.panelWidths(w)
	bottom := uiBottomRows

	a.rebuildCurrent()
	a.rebuildSidebar()

	a.sidebar.SetRect(0, 0, left, h-1)
	topicHeight := topicWidgetHeight(a.topic.Text, w-right-left)
	a.topic.SetRect(left, 0, w-right, topicHeight)
	a.transcript.SetRect(left, topicHeight, w-right, h-bottom)
	a.input.SetRect(left, h-bottom, w, h-1)

	relayWidth := a.relayReconnectControlWidth()
	statusRight := w
	if relayWidth > 0 {
		// Leave at least one cell for the ordinary status region on unusually
		// narrow terminals. render() already rejects widths below 40.
		if relayWidth >= w {
			relayWidth = w - 1
		}
		statusRight = w - relayWidth
		a.relayControl.Text = " " + a.relayReconnectLabel() + " "
		a.relayControl.SetRect(statusRight, h-1, w, h)
	}
	a.status.SetRect(0, h-1, statusRight, h)

	items := []ui.Drawable{a.topic, a.transcript, a.input, a.status}
	if left > 0 {
		items = append(items, a.sidebar)
	}
	if relayWidth > 0 {
		items = append(items, a.relayControl)
	}
	if right > 0 {
		a.users.SetRect(w-right, 0, w, h-bottom)
		a.refreshUserRows(userListVisibleRows(h))
		items = append(items, a.users)
	}

	// A normal gotui render is incremental. After a script reload we need a
	// physical-screen resync, not just a logical clear: terminal output from a
	// script or stale tcell cells can otherwise survive until some later event
	// happens to repaint that row. Clear the logical screen immediately before
	// the rebuilt frame and then ask tcell to rewrite every visible cell.
	forceSync := a.forceScreenSync && ui.DefaultBackend.Screen != nil
	if forceSync {
		ui.DefaultBackend.Screen.Clear()
	}
	ui.Render(items...)
	if forceSync {
		ui.DefaultBackend.Screen.Sync()
		a.forceScreenSync = false
	}
}

func (a *App) rebuildSidebar() {
	buffers, current := a.state.SnapshotInfo()
	servers := a.irc.ServerNames()
	var rows []string
	var keys []string
	selected := 0
	numberWidth := len(strconv.Itoa(len(buffers)))
	if numberWidth < 1 {
		numberWidth = 1
	}
	numberLabel := func(n int) string {
		return styled(fmt.Sprintf("%*d", numberWidth, n), a.cfg.Theme.Muted) + " "
	}
	for _, server := range servers {
		serverKey := model.Key(server, "*server*")
		serverIcon := styled("◆", a.cfg.Theme.Server)
		if !a.irc.IsConnected(server) {
			if a.irc.WantsConnection(server) {
				serverIcon = styled("◌", a.cfg.Theme.Notice)
			} else {
				serverIcon = styled("◇", a.cfg.Theme.Muted)
			}
		}
		label := numberLabel(len(rows)+1) + serverIcon + " " + server
		if serverKey == current {
			selected = len(rows)
		}
		rows = append(rows, label)
		keys = append(keys, serverKey)
		for _, b := range buffers {
			if b.Server != server || b.Target == "*server*" {
				continue
			}
			prefix := "  " + styled("·", a.cfg.Theme.Muted) + " "
			if model.IsChannel(b.Target) && a.irc.IsJoined(b.Server, b.Target) {
				prefix = "  " + styled("✓", a.cfg.Theme.Channel) + " "
			} else if model.IsChannel(b.Target) && a.irc.IsConnected(b.Server) && a.irc.WantsConnection(b.Server) {
				prefix = "  " + styled("…", a.cfg.Theme.Notice) + " "
			} else if !model.IsChannel(b.Target) {
				prefix = "  " + styled("@", a.cfg.Theme.Query) + " "
			}
			label := numberLabel(len(rows)+1) + prefix + b.Target
			if b.Unread > 0 {
				label += " " + styled(fmt.Sprintf("(%d)", b.Unread), a.cfg.Theme.Unread)
			}
			key := model.Key(b.Server, b.Target)
			if key == current {
				selected = len(rows)
			}
			rows = append(rows, label)
			keys = append(keys, key)
		}
	}
	a.sidebar.Rows = rows
	a.sidebarKeys = keys
	a.sidebar.SelectedRow = selected
}

// saveTranscriptCache remembers the rendered state of a buffer when the user
// leaves it. Rows are copied so later widget mutations cannot rewrite another
// buffer's cached transcript.
func (a *App) saveTranscriptCache(key string, cached transcriptCache) {
	if key == "" {
		return
	}
	if a.transcriptCaches == nil {
		a.transcriptCaches = make(map[string]transcriptCache)
	}
	cached.rows = append([]string(nil), cached.rows...)
	a.transcriptCaches[key] = cached
}

// restoreTranscriptCache restores a buffer's existing live transcript. The
// persistent log backlog is therefore loaded only on the first visit; later
// visits keep the original muted seed plus every live row collected since.
func (a *App) restoreTranscriptCache(key string) bool {
	cached, ok := a.transcriptCaches[key]
	if !ok {
		return false
	}
	a.transcript.Rows = append([]string(nil), cached.rows...)
	a.transcriptStart = cached.start
	a.transcriptTotal = cached.total
	a.transcriptFromLog = cached.fromLog
	a.transcriptBacklogRows = cached.backlogRows
	return true
}

func (a *App) rebuildCurrent() {
	b := a.state.CurrentInfo()
	if b == nil {
		a.transcript.Rows = []string{"No buffer selected"}
		a.transcriptKey = ""
		a.transcriptReset = false
		a.transcriptStart = 0
		a.transcriptTotal = 0
		a.transcriptFromLog = false
		a.transcriptBacklogRows = 0
		a.users.Rows = nil
		a.userNicks = nil
		a.userScroll = 0
		a.topic.Text = ""
		a.input.Title = "Message"
		a.status.Text = "Copperline"
		return
	}

	key := model.Key(b.Server, b.Target)
	bufferChanged := key != a.transcriptKey
	resetWidget := bufferChanged || a.transcriptReset
	preservedSelected := -1
	if resetWidget {
		if bufferChanged {
			a.userScroll = 0
		}
		oldRows := append([]string(nil), a.transcript.Rows...)
		oldSelected := a.transcript.SelectedRow
		oldStart := a.transcriptStart
		oldTotal := a.transcriptTotal
		oldFromLog := a.transcriptFromLog
		oldBacklogRows := a.transcriptBacklogRows
		preserveSelection := !bufferChanged && a.transcriptReset && !a.follow

		// Save the buffer we are leaving. The persistent log preview is only a
		// one-time seed; revisiting a buffer must restore its live transcript
		// rather than replacing everything with the newest ten log lines.
		if bufferChanged && a.transcriptKey != "" {
			a.saveTranscriptCache(a.transcriptKey, transcriptCache{
				rows:        oldRows,
				selectedRow: oldSelected,
				start:       oldStart,
				total:       oldTotal,
				fromLog:     oldFromLog,
				backlogRows: oldBacklogRows,
			})
		}

		// gotui's List keeps its vertical topRow offset internally. Rebuilding
		// the widget clears stale private scroll state on buffer changes/resizes.
		a.transcript = a.newTranscriptList()
		a.transcriptKey = key
		a.transcriptReset = false

		if bufferChanged {
			a.follow = true
			a.search = searchState{}
			if !a.restoreTranscriptCache(key) {
				a.transcriptStart = 0
				a.transcriptTotal = 0
				a.transcriptFromLog = false
				a.transcriptBacklogRows = 0

				// Seed conversational buffers (channels and private-message queries)
				// from disk only on their first visit. Later visits restore the
				// cached live transcript above.
				a.loadConversationLogBacklog(b)
			}
		} else {
			// A resize should not change what the user was reading. Carry the
			// existing rendered rows into a fresh widget while clearing gotui's
			// hidden scroll offset.
			a.transcript.Rows = oldRows
			a.transcriptStart = oldStart
			a.transcriptTotal = oldTotal
			a.transcriptFromLog = oldFromLog
			a.transcriptBacklogRows = oldBacklogRows
			if preserveSelection {
				preservedSelected = oldSelected
			}
		}
	}

	// Pull only messages that arrived after the rows currently cached by the
	// TUI. A first visit with persistent backlog starts at total zero so all
	// retained messages from this Copperline session are rendered as live rows.
	window := a.state.CurrentWindow(a.transcriptTotal)
	if bufferChanged && !a.transcriptFromLog {
		window = a.state.CurrentWindow(0)
	}
	if window == nil {
		return
	}

	a.input.Title, a.input.Placeholder = a.inputDisplayState(b)

	if a.transcriptFromLog {
		// If a buffer was left inactive long enough that some unseen messages
		// have already fallen out of the retained in-memory window, do not reload
		// its log preview and recolor/truncate the existing live transcript. Keep
		// what the user already saw and append the retained live window.
		if a.transcriptTotal < window.Start {
			for _, msg := range window.Messages {
				a.transcript.Rows = append(a.transcript.Rows, a.theme.formatMessage(msg, a.cfg.General.Timestamp))
			}
			a.transcriptStart = window.Start
			a.transcriptTotal = window.Total
			window.Messages = nil
		} else if a.transcriptTotal > window.Total {
			// Defensive recovery for an impossible/stale counter: retain the visible
			// transcript and resume from the current buffer total.
			a.transcriptStart = window.Start
			a.transcriptTotal = window.Total
			window.Messages = nil
		}

		if a.transcriptFromLog {
			if len(window.Messages) > 0 && len(a.transcript.Rows) == 1 && a.transcript.Rows[0] == "No messages yet." {
				a.transcript.Rows = nil
			}
			for _, msg := range window.Messages {
				a.transcript.Rows = append(a.transcript.Rows, a.theme.formatMessage(msg, a.cfg.General.Timestamp))
			}
			a.transcriptTotal = window.Total

			// Keep the rendered list bounded even if a channel stays selected for a
			// long session. The persistent log still contains everything.
			maxRows := a.cfg.General.HistoryLines + a.transcriptBacklogRows
			if maxRows > 0 && len(a.transcript.Rows) > maxRows {
				drop := len(a.transcript.Rows) - maxRows
				a.transcript.Rows = append([]string(nil), a.transcript.Rows[drop:]...)
				if !a.follow {
					a.transcript.SelectedRow -= drop
				}
			}
		}
	}

	if !a.transcriptFromLog {
		// A resize/copy-mode exit rebuilds the gotui List object but carries the
		// existing rendered rows forward. Do not treat that widget reset as a
		// history reset: CurrentWindow(transcriptTotal) intentionally returns only
		// newer messages, which is often empty and used to erase the carried rows.
		fullReset := bufferChanged ||
			a.transcriptTotal < window.Start ||
			a.transcriptTotal > window.Total ||
			a.transcriptStart > window.Start ||
			(a.transcriptTotal == 0 && window.Total > 0)

		if window.Total == 0 {
			a.transcript.Rows = []string{"No messages yet."}
			a.transcriptStart = 0
			a.transcriptTotal = 0
		} else if fullReset {
			rows := make([]string, 0, len(window.Messages))
			for _, msg := range window.Messages {
				rows = append(rows, a.theme.formatMessage(msg, a.cfg.General.Timestamp))
			}
			a.transcript.Rows = rows
			a.transcriptStart = window.Start
			a.transcriptTotal = window.Total
		} else {
			// If MaxLines evicted old messages, discard the matching cached rows.
			if window.Start > a.transcriptStart {
				drop := int(window.Start - a.transcriptStart)
				if drop >= len(a.transcript.Rows) {
					a.transcript.Rows = nil
				} else {
					a.transcript.Rows = a.transcript.Rows[drop:]
				}
				if !a.follow {
					a.transcript.SelectedRow -= drop
				}
			}
			for _, msg := range window.Messages {
				a.transcript.Rows = append(a.transcript.Rows, a.theme.formatMessage(msg, a.cfg.General.Timestamp))
			}
			a.transcriptStart = window.Start
			a.transcriptTotal = window.Total
		}
	}

	a.transcript.Title = b.Server + " / " + b.Target
	// gotui v5.0.3 List.ScrollBottom sets SelectedRow to len(Rows)-1. On an
	// empty list that becomes -1, and Draw can copy it into the private topRow
	// offset before indexing Rows[-1]. Keep the transcript non-empty at every
	// render boundary, even if a future state/window edge case produces no rows.
	if len(a.transcript.Rows) == 0 {
		a.transcript.Rows = []string{"No messages yet."}
	}
	if preservedSelected >= 0 {
		a.transcript.SelectedRow = preservedSelected
	}
	if a.transcript.SelectedRow >= len(a.transcript.Rows) {
		a.transcript.SelectedRow = len(a.transcript.Rows) - 1
	}
	if a.transcript.SelectedRow < 0 {
		a.transcript.SelectedRow = 0
	}
	if a.follow {
		a.scrollTranscriptBottom()
		a.state.MarkReadThrough(b.Server, b.Target, window.Total)
	}
	a.updateCatchup(window.BufferInfo, bufferChanged)

	topic := ""
	if model.IsChannel(b.Target) {
		topic = a.irc.ChannelTopic(b.Server, b.Target)
	}
	a.topic.Text = topic

	a.userNicks = a.irc.Names(b.Server, b.Target)
	a.userScroll = clampUserScroll(a.userScroll, len(a.userNicks), userListVisibleRowsForCurrentTerminal())

	nick := a.irc.CurrentNick(b.Server)
	caps := len(a.irc.Capabilities(b.Server))
	offers := len(a.irc.DCCOffers())
	joinState := ""
	if model.IsChannel(b.Target) {
		if a.irc.IsJoined(b.Server, b.Target) {
			joinState = "  joined"
		} else {
			joinState = "  NOT JOINED"
		}
	}
	if a.jumpMode {
		digits := a.jumpDigits
		if digits == "" {
			digits = "_"
		} else {
			digits += "_"
		}
		a.status.Text = fmt.Sprintf(" Jump to buffer: %s  Enter select  %s cancel ", digits, a.cfg.Keybindings.JumpCancel)
		return
	}
	if a.relayReconnecting.Load() {
		a.status.Text = " Re-establishing Copperline SSH relay connection - please wait "
		return
	}
	if loading, progress := a.startupLoading(); loading {
		phase := "Loading IRC state - please wait"
		if progress.connectedServers < progress.totalServers {
			phase = "Connecting to IRC servers - please wait"
		} else if progress.joinedChannels < progress.totalChannels {
			phase = "Joining configured channels - please wait"
		}
		a.status.Text = fmt.Sprintf(" STARTING %s  servers %d/%d  channels %d/%d  %s ",
			startupSpinner(time.Now()),
			progress.connectedServers, progress.totalServers,
			progress.joinedChannels, progress.totalChannels,
			phase)
		return
	}
	a.status.Text = fmt.Sprintf(" %s  %s%s  nick:%s  IRCv3:%d caps  DCC:%d  %s/%s buffers  %s/%s users  %s/%s history  %s jump  %s/%s scroll  /help ",
		b.Server, b.Target, joinState, nick, caps, offers,
		a.cfg.Keybindings.NextBuffer, a.cfg.Keybindings.PreviousBuffer,
		a.cfg.Keybindings.UserListDown, a.cfg.Keybindings.UserListUp,
		a.cfg.Keybindings.HistoryPrevious, a.cfg.Keybindings.HistoryNext,
		a.cfg.Keybindings.JumpBuffer,
		a.cfg.Keybindings.TranscriptPageUp, a.cfg.Keybindings.TranscriptPageDown)
}

// loadConversationLogBacklog initializes a newly selected conversational
// buffer (channel or private-message query) from the last few plaintext log
// lines from before this Copperline session. Server buffers are deliberately
// excluded. Returning true means the caller should keep that muted context and
// then render all retained current-session messages from model.State using
// normal live styling.
func (a *App) loadConversationLogBacklog(b *model.BufferInfo) bool {
	if b == nil || b.Target == "*server*" || !a.cfg.General.LoggingEnabled() {
		return false
	}
	n := a.cfg.General.LogBacklogLinesValue()
	if n <= 0 {
		return false
	}

	// Pin the boundary on first view even if this channel/query has not received
	// a message yet. If it has received traffic already, onMessage pinned the
	// same boundary before exposing that traffic to state, so this is a no-op.
	if err := a.logger.BeginBuffer(b.Server, b.Target); err != nil {
		return false
	}
	lines, err := a.logger.BacklogTail(b.Server, b.Target, n)
	if err != nil {
		return false
	}
	if len(lines) == 0 && b.Total > 0 {
		// If logging failed or has not caught up for some reason, preserve the
		// existing in-memory behavior instead of hiding available history.
		return false
	}

	if len(lines) == 0 {
		a.transcript.Rows = []string{"No messages yet."}
	} else {
		rows := make([]string, 0, len(lines))
		for _, line := range lines {
			rows = append(rows, a.theme.formatLogBacklog(line))
		}
		a.transcript.Rows = rows
	}
	a.transcriptFromLog = true
	a.transcriptBacklogRows = len(lines)
	// State totals start at zero for each Copperline process. Starting the live
	// cursor at zero is intentional: a channel or PM query may have accumulated
	// messages for hours before the user first opens it, and those messages are
	// still current-session traffic, not muted persistent history.
	a.transcriptStart = 0
	a.transcriptTotal = 0
	return true
}

// Use the same panel geometry for drawing and mouse hit testing.
func (a *App) panelWidths(width int) (left, right int) {
	if !a.channelListHidden {
		left = 28
		if width < 70 {
			left = 20
		}
	}
	if !a.userListHidden && width > 90 {
		right = userPaneWidth
	}
	return
}

func (a *App) panelLayoutChanged() {
	a.clearNickClick()
	a.transcriptReset = true
	a.forceScreenSync = true
}

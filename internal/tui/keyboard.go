package tui

import (
	"fmt"
	"strings"
	"time"

	"copperline/internal/config"
	"copperline/internal/model"

	"github.com/gdamore/tcell/v3"
	ui "github.com/metaspartan/gotui/v5"
)

func (a *App) handleUIEvent(e ui.Event) {
	// In bare/copy mode the terminal owns the mouse and Copperline leaves the
	// displayed transcript frozen. Only the toggle, quit, and resize need to be
	// handled until the normal interface is restored.
	if a.copyMode {
		if e.Type == ui.KeyboardEvent {
			if config.KeyBindingMatches(a.cfg.Keybindings.CopyMode, e.ID) {
				a.setCopyMode(false)
				return
			}
			if config.KeyBindingMatches(a.cfg.Keybindings.Quit, e.ID) {
				a.stopped.Store(true)
			}
			return
		}
		if e.Type == ui.ResizeEvent {
			a.renderCopyMode()
		}
		return
	}

	switch e.Type {
	case ui.ResizeEvent:
		a.resetNickCompletion()
		// gotui's List keeps its vertical topRow offset internally. A resize
		// changes the number of visible rows without recalculating that offset,
		// which can leave only the final message visible until the user scrolls.
		// Rebuild the transcript widget on the next render to clear that stale
		// private scroll state. rebuildCurrent preserves the selected row when
		// the user was intentionally reading scrollback.
		a.transcriptReset = true
		return
	case ui.MouseEvent:
		a.resetNickCompletion()
		if a.cfg.General.Mouse {
			a.handleMouse(e)
		}
		a.syncOutgoingTyping()
		return
	case ui.KeyboardEvent:
		if delta := userListScrollEventDelta(e, a.cfg.Keybindings); delta != 0 {
			a.resetNickCompletion()
			a.scrollUsers(delta)
			a.syncOutgoingTyping()
			return
		}
		a.handleKey(e)
	}
}

// keyBindingMatchesEvent normally matches gotui's event ID. Modern terminal
// keyboard protocols can also report Ctrl+letter as a KeyRune carrying a Ctrl
// modifier. gotui v5.0.3 preserves Alt on KeyRune IDs but not Ctrl, so inspect
// the raw tcell event as a fallback instead of losing configurable Ctrl keys.
func keyBindingMatchesEvent(binding string, e ui.Event) bool {
	if config.KeyBindingMatches(binding, e.ID) {
		return true
	}

	keyEvent, ok := e.Payload.(*tcell.EventKey)
	if !ok || keyEvent == nil || keyEvent.Key() != tcell.KeyRune {
		return false
	}

	ctrl := keyEvent.Modifiers()&tcell.ModCtrl != 0
	alt := keyEvent.Modifiers()&tcell.ModAlt != 0
	return keyBindingMatchesRuneFallback(binding, keyEvent.Str(), ctrl, alt)
}

func keyBindingMatchesRuneFallback(binding, text string, ctrl, alt bool) bool {
	fallback := canonicalRuneKeyEvent(text, ctrl, alt)
	return fallback != "" && config.KeyBindingMatches(binding, fallback)
}

func canonicalRuneKeyEvent(text string, ctrl, alt bool) string {
	runes := []rune(text)
	if len(runes) != 1 {
		return ""
	}
	r := runes[0]

	// Some legacy reports put the ASCII control byte in Str() instead of the
	// printable letter. Turn Ctrl+A..Ctrl+Z back into a..z for matching.
	if ctrl && r >= 1 && r <= 26 {
		r = 'a' + r - 1
	}

	switch {
	case ctrl:
		return fmt.Sprintf("<C-%c>", r)
	case alt:
		return fmt.Sprintf("<M-%c>", r)
	default:
		return string(r)
	}
}

func userListScrollEventDelta(e ui.Event, keys config.KeybindingsConfig) int {
	switch {
	case keyBindingMatchesEvent(keys.UserListDown, e):
		return 1
	case keyBindingMatchesEvent(keys.UserListUp, e):
		return -1
	default:
		return 0
	}
}

func userListScrollKeyDelta(id string, keys config.KeybindingsConfig) int {
	switch {
	case config.KeyBindingMatches(keys.UserListDown, id):
		return 1
	case config.KeyBindingMatches(keys.UserListUp, id):
		return -1
	default:
		return 0
	}
}

func (a *App) handleKey(e ui.Event) {
	id := e.ID
	beforeText := a.input.Text
	historyNavigation := false
	defer func() {
		if a.input.Text != beforeText {
			a.typingLastEdit = time.Now()
			if !historyNavigation {
				a.resetInputHistoryNavigation()
			}
		}
		a.syncOutgoingTyping()
	}()

	keys := a.cfg.Keybindings
	matches := func(binding string) bool { return keyBindingMatchesEvent(binding, e) }
	if !matches(keys.CompleteNick) {
		a.resetNickCompletion()
	}

	if a.jumpMode {
		if a.handleJumpKey(e) {
			return
		}
	}

	// Configurable navigation/action keys are checked before the fixed input
	// editing keys. Configuration validation reserves the essential editing keys
	// so navigation cannot make the input field unusable.
	switch {
	case a.relayReconnectEnabled() && matches(keys.RelayReconnect):
		a.requestRelayReconnect()
		return
	case matches(keys.CopyMode):
		a.setCopyMode(true)
		return
	case matches(keys.Quit):
		a.stopped.Store(true)
		return
	case matches(keys.ClearInput):
		a.input.Text = ""
		a.input.Cursor = 0
		return
	case matches(keys.NextBuffer):
		a.selectRelative(1)
		return
	case matches(keys.NextUnread):
		a.selectNextUnread()
		return
	case matches(keys.SearchNext):
		a.moveSearch(1)
		return
	case matches(keys.SearchPrevious):
		a.moveSearch(-1)
		return
	case matches(keys.PreviousBuffer):
		a.selectRelative(-1)
		return
	case matches(keys.JumpBuffer):
		a.jumpMode = true
		a.jumpDigits = ""
		return
	case matches(keys.CompleteNick):
		a.completeNick()
		return
	case matches(keys.HistoryPrevious):
		historyNavigation = true
		a.inputHistoryPrevious()
		return
	case matches(keys.HistoryNext):
		historyNavigation = true
		a.inputHistoryNext()
		return
	case matches(keys.TranscriptPageUp):
		a.follow = false
		a.transcript.ScrollPageUp()
		return
	case matches(keys.TranscriptPageDown):
		a.transcript.ScrollPageDown()
		a.resumeFollowAtBottom()
		return
	case matches(keys.TranscriptLineUp):
		a.follow = false
		a.transcript.ScrollUp()
		return
	case matches(keys.TranscriptLineDown):
		a.transcript.ScrollDown()
		a.resumeFollowAtBottom()
		return
	case matches(keys.FollowBottom):
		a.input.Cursor = len([]rune(a.input.Text))
		a.follow = true
		a.scrollTranscriptBottom()
		return
	}

	switch id {
	case "<Enter>":
		line := strings.TrimSpace(a.input.Text)
		if line == "" {
			return
		}

		// Normal chat is safety-sensitive in relay mode. Do not erase the user's
		// text until the backend has acknowledged the send request. Previously the
		// input was cleared first, so a half-dead SSH attachment could silently eat
		// a message while Copperline was still displaying cached IRC state.
		if !strings.HasPrefix(line, "/") {
			b := a.state.CurrentInfo()
			if b == nil {
				return
			}
			if reason := a.chatWaitReason(b.Server, b.Target); reason != "" {
				a.local(b.Server, b.Target, model.KindSystem, reason)
				return
			}

			a.follow = true
			a.scrollTranscriptBottom()
			if err := a.irc.SendMessage(b.Server, b.Target, line); err != nil {
				if a.cfg.Relay.ModeValue() == "client" {
					a.local(b.Server, b.Target, model.KindError, "Relay send was not confirmed; message kept in input. Check whether it arrived before retrying: "+err.Error())
				} else {
					a.local(b.Server, b.Target, model.KindError, "Send failed; message kept in input: "+err.Error())
				}
				return
			}

			a.addInputHistory(line)
			a.input.Text = ""
			a.input.Cursor = 0
			return
		}

		if handled, err := a.sendTextCommand(line); handled {
			if err != nil {
				if b := a.state.CurrentInfo(); b != nil {
					a.local(b.Server, b.Target, model.KindError, "Send was not confirmed; command kept in input. Check before retrying: "+err.Error())
				}
				return
			}
			a.addInputHistory(line)
			a.input.Text = ""
			a.input.Cursor = 0
			return
		}

		// Other commands may intentionally mutate local UI state.
		a.addInputHistory(line)
		a.input.Text = ""
		a.input.Cursor = 0
		a.execute(line)
	case "<Backspace>", "<C-h>":
		a.input.Backspace()
	case "<Left>":
		a.input.MoveCursorLeft()
	case "<Right>":
		a.input.MoveCursorRight()
	case "<Home>":
		a.input.Cursor = 0
	case "<End>":
		// If follow_bottom is rebound, End retains its normal input-editing role.
		a.input.Cursor = len([]rune(a.input.Text))
	case "<Space>":
		a.input.InsertRune(' ')
	default:
		for _, r := range printableInputText(e) {
			a.input.InsertRune(r)
		}
	}
}

// printableInputText returns text for an ordinary printable keyboard event.
// gotui uses angle-bracket strings such as <Enter> for special keys, but a
// literal '<' rune also has an event ID beginning with '<'. Prefer the raw
// tcell KeyRune payload so user input is never mistaken for gotui notation.
func printableInputText(e ui.Event) string {
	if keyEvent, ok := e.Payload.(*tcell.EventKey); ok && keyEvent != nil && keyEvent.Key() == tcell.KeyRune {
		// Ctrl/Alt rune events are shortcuts, not text input. Shift is allowed:
		// terminals commonly report characters such as '<' as Shift+',' while
		// Str() already contains the printable result.
		if keyEvent.Modifiers()&(tcell.ModCtrl|tcell.ModAlt) != 0 {
			return ""
		}
		return keyEvent.Str()
	}

	// Keep a small fallback for terminals/backends that do not expose the raw
	// tcell event payload. '<' itself is printable; other leading-angle IDs are
	// gotui special-key notation.
	if e.ID == "<" {
		return "<"
	}
	if !strings.HasPrefix(e.ID, "<") {
		return e.ID
	}
	return ""
}

// inputHistoryCurrent returns the session history for the active buffer. Input
// history is deliberately per-buffer so recalling an old private message in a
// public channel cannot happen by accident.
func (a *App) inputHistoryCurrent() *inputHistoryState {
	b := a.state.CurrentInfo()
	if b == nil {
		return nil
	}
	key := model.Key(b.Server, b.Target)
	if a.inputHistory == nil {
		a.inputHistory = make(map[string]*inputHistoryState)
	}
	h := a.inputHistory[key]
	if h == nil {
		h = &inputHistoryState{}
		a.inputHistory[key] = h
	}
	return h
}

func (a *App) inputHistoryLimit() int {
	if a.cfg == nil {
		return 10
	}
	return a.cfg.General.InputHistoryLimitValue()
}

func (a *App) addInputHistory(line string) {
	h := a.inputHistoryCurrent()
	limit := a.inputHistoryLimit()
	if h == nil || line == "" || limit == 0 {
		return
	}
	if len(h.entries) == 0 || h.entries[len(h.entries)-1] != line {
		h.entries = append(h.entries, line)
		if len(h.entries) > limit {
			drop := len(h.entries) - limit
			h.entries = append([]string(nil), h.entries[drop:]...)
		}
	}
	h.index = len(h.entries)
	h.draft = ""
	h.active = false
}

func (a *App) inputHistoryPrevious() {
	h := a.inputHistoryCurrent()
	if h == nil || len(h.entries) == 0 {
		return
	}
	if !h.active {
		h.draft = a.input.Text
		h.index = len(h.entries)
		h.active = true
	}
	if h.index > 0 {
		h.index--
	}
	a.setInputText(h.entries[h.index])
}

func (a *App) inputHistoryNext() {
	h := a.inputHistoryCurrent()
	if h == nil || !h.active {
		return
	}
	if h.index < len(h.entries)-1 {
		h.index++
		a.setInputText(h.entries[h.index])
		return
	}

	// Moving past the newest history entry returns to exactly what the user was
	// typing before they first pressed History Previous.
	draft := h.draft
	h.index = len(h.entries)
	h.draft = ""
	h.active = false
	a.setInputText(draft)
}

func (a *App) setInputText(text string) {
	a.input.Text = text
	a.input.Cursor = len([]rune(text))
	a.resetNickCompletion()
}

// Any real edit leaves history-navigation mode. The entries remain available;
// only the temporary cursor/draft state is reset.
func (a *App) resetInputHistoryNavigation() {
	for _, h := range a.inputHistory {
		if h == nil {
			continue
		}
		h.index = len(h.entries)
		h.draft = ""
		h.active = false
	}
}

// setCopyMode implements a WeeChat-style bare display. The normal widget
// layout is replaced by a borderless full-screen snapshot of the current
// transcript and terminal mouse reporting is disabled so the terminal can do
// native click-and-drag selection. While active, the main loop intentionally
// does not redraw the screen; network activity continues in the background.

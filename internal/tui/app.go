package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"copperline/internal/config"
	gotifynotify "copperline/internal/gotify"
	ircclient "copperline/internal/irc"
	"copperline/internal/logging"
	"copperline/internal/model"
	"copperline/internal/scripting"

	ui "github.com/metaspartan/gotui/v5"
	"github.com/metaspartan/gotui/v5/widgets"
)

type App struct {
	cfg     *config.Config
	state   *model.State
	irc     *ircclient.Manager
	logger  *logging.Logger
	gotify  *gotifynotify.Notifier
	scripts *scripting.Engine
	theme   uiTheme
	stopped atomic.Bool

	sidebar    *widgets.List
	transcript *widgets.List
	users      *widgets.List
	topic      *widgets.Paragraph
	input      *widgets.Input
	status     *widgets.Paragraph

	sidebarKeys     []string
	userNicks       []string
	follow          bool
	transcriptKey   string
	transcriptReset bool

	nickCompletionMatches []string
	nickCompletionIndex   int
	nickCompletionStart   int
	nickCompletionEnd     int
	nickCompletionFirst   bool

	jumpMode   bool
	jumpDigits string

	typingServer    string
	typingTarget    string
	typingSentState string
	typingLastSent  time.Time
	typingLastEdit  time.Time

	lastNickClickServer string
	lastNickClickNick   string
	lastNickClickAt     time.Time

	copyMode bool
}

func New(cfg *config.Config) *App {
	a := &App{
		cfg:    cfg,
		state:  model.New(cfg.General.HistoryLines),
		logger: logging.New(cfg.General.LoggingEnabled(), cfg.General.LogDir, cfg.General.Timestamp),
		gotify: gotifynotify.New(cfg.Gotify),
		theme:  newUITheme(cfg.Theme),
		follow: true,
	}
	for _, s := range cfg.Servers {
		a.state.Ensure(s.Name, "*server*")
		for _, ch := range s.Channels {
			a.state.Ensure(s.Name, ch)
		}
	}
	a.irc = ircclient.New(cfg, a.onMessage)
	if cfg.Scripting.EnabledValue() {
		a.scripts = scripting.New(config.ExpandPath(cfg.Scripting.Dir), scripting.Host{
			Print: func(server, target, text string) {
				a.local(server, target, model.KindSystem, text)
			},
			Message: func(server, target, text string) error {
				return a.irc.SendMessage(server, target, text)
			},
			Notice: func(server, target, text string) error {
				return a.irc.Notice(server, target, text)
			},
			Raw: func(server, line string) error {
				return a.irc.Raw(server, line)
			},
			Active: func() scripting.Context {
				b := a.state.Current()
				if b == nil {
					return scripting.Context{}
				}
				return scripting.Context{Server: b.Server, Target: b.Target, Nick: a.irc.CurrentNick(b.Server)}
			},
			CurrentNick: a.irc.CurrentNick,
		})
		a.irc.SetEventSink(a.onIRCEvent)
	}
	a.makeWidgets()
	return a
}

func (a *App) makeWidgets() {
	a.sidebar = widgets.NewList()
	a.sidebar.Title = "Copperline"
	a.sidebar.WrapText = false
	a.sidebar.BorderRounded = true
	a.theme.applyBlock(a.sidebar)

	a.transcript = a.newTranscriptList()

	a.users = widgets.NewList()
	a.users.Title = "Users"
	a.users.WrapText = false
	a.users.BorderRounded = true
	a.theme.applyBlock(a.users)
	a.users.SelectedStyle = a.users.TextStyle

	a.topic = widgets.NewParagraph()
	a.topic.Title = "Topic"
	a.topic.WrapText = false
	a.topic.BorderRounded = true
	a.theme.applyParagraph(a.topic, a.theme.topic)

	a.input = widgets.NewInput()
	a.input.Title = "Message"
	a.input.Placeholder = "Type a message or /help"
	a.input.BorderRounded = true
	a.theme.applyInput(a.input)

	a.status = widgets.NewParagraph()
	a.status.Border = false
	a.status.WrapText = false
	a.theme.applyStatus(a.status)
}

func (a *App) newTranscriptList() *widgets.List {
	list := widgets.NewList()
	list.Title = "Messages"
	list.WrapText = true
	list.BorderRounded = true
	a.theme.applyBlock(list)
	list.SelectedStyle = list.TextStyle
	return list
}

func (a *App) Run() error {
	if err := ui.Init(); err != nil {
		return err
	}
	defer ui.Close()

	if a.scripts != nil {
		defer a.scripts.Close()
		loaded, err := a.scripts.Reload()
		b := a.state.Current()
		server, target := "", "*server*"
		if b != nil {
			server, target = b.Server, b.Target
		}
		if err != nil {
			a.local(server, target, model.KindError, "Lua: "+err.Error())
		} else if len(loaded) > 0 {
			a.local(server, target, model.KindSystem, fmt.Sprintf("Lua: loaded %d script(s)", len(loaded)))
		}
	}
	defer a.irc.Stop("Copperline exiting")
	a.irc.Start()
	a.render()
	events := ui.PollEvents()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for !a.stopped.Load() {
		select {
		case e := <-events:
			a.handleUIEvent(e)
			if !a.copyMode {
				a.render()
			}
		case <-ticker.C:
			a.syncOutgoingTyping()
			// Bare/copy mode deliberately freezes the terminal display so
			// native text selection is not disturbed by periodic redraws.
			// IRC state, logging, Gotify, DCC, and timers continue normally.
			if !a.copyMode {
				a.render()
			}
		}
	}
	return nil
}

func (a *App) onMessage(msg model.Message) {
	// Mentions are detected only for incoming chat/action messages. In
	// particular, we never inspect the input widget, and an echoed copy of
	// our own outgoing message cannot highlight itself as a mention.
	self := ""
	if msg.Kind == model.KindMessage || msg.Kind == model.KindAction {
		self = a.irc.CurrentNick(msg.Server)
		if self != "" && msg.Nick != "" && !strings.EqualFold(msg.Nick, self) && containsNickMention(msg.Text, self) {
			msg.Mention = true
		}
	}
	a.state.Add(msg)
	_ = a.logger.Write(msg)
	a.notifyGotify(msg, self)
	if a.scripts != nil && (msg.Kind == model.KindMessage || msg.Kind == model.KindAction || msg.Kind == model.KindDCC) {
		a.scripts.EmitEvent(scripting.Event{
			Time:    msg.Time,
			Server:  msg.Server,
			Command: string(msg.Kind),
			Source:  msg.Nick,
			Params:  []string{msg.Target, msg.Text},
			Tags:    msg.Tags,
		})
	}
}

func (a *App) onIRCEvent(ev ircclient.Event) {
	if a.scripts == nil {
		return
	}
	a.scripts.EmitEvent(scripting.Event{
		Time:    ev.Time,
		Server:  ev.Server,
		Command: ev.Command,
		Source:  ev.Source,
		Params:  ev.Params,
		Tags:    ev.Tags,
		Raw:     true,
	})
}

func (a *App) notifyGotify(msg model.Message, self string) {
	if a.gotify == nil || !a.gotify.Enabled() {
		return
	}
	if msg.Kind != model.KindMessage && msg.Kind != model.KindAction {
		return
	}
	if self == "" {
		self = a.irc.CurrentNick(msg.Server)
	}
	if self == "" || msg.Nick == "" || strings.EqualFold(msg.Nick, self) {
		return
	}

	body := fmt.Sprintf("<%s> %s", msg.Nick, msg.Text)
	if msg.Kind == model.KindAction {
		body = fmt.Sprintf("* %s %s", msg.Nick, msg.Text)
	}

	// A private message which also contains our nick generates only one
	// notification: the higher-signal mention notification wins.
	if msg.Mention && a.cfg.Gotify.MentionsEnabled() {
		a.gotify.Send(fmt.Sprintf("Copperline mention: %s / %s", msg.Server, msg.Target), body)
		return
	}

	private := msg.Target != "" && msg.Target != "*server*" && !model.IsChannel(msg.Target)
	if private && a.cfg.Gotify.PrivateMessagesEnabled() {
		a.gotify.Send(fmt.Sprintf("Copperline PM: %s on %s", msg.Nick, msg.Server), body)
	}
}

func (a *App) local(server, target string, kind model.Kind, text string) {
	if server == "" {
		servers := a.irc.ServerNames()
		if len(servers) == 0 {
			return
		}
		server = servers[0]
	}
	if target == "" {
		target = "*server*"
	}
	a.onMessage(model.Message{Time: time.Now(), Server: server, Target: target, Kind: kind, Text: text})
}

func (a *App) handleUIEvent(e ui.Event) {
	// In bare/copy mode the terminal owns the mouse and Copperline leaves the
	// displayed transcript frozen. Only the toggle, quit, and resize need to be
	// handled until the normal interface is restored.
	if a.copyMode {
		if e.Type == ui.KeyboardEvent {
			if isCopyModeKey(e.ID) {
				a.setCopyMode(false)
				return
			}
			if e.ID == "<C-c>" {
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
		a.handleKey(e.ID)
	}
}

func (a *App) handleKey(id string) {
	beforeText := a.input.Text
	defer func() {
		if a.input.Text != beforeText {
			a.typingLastEdit = time.Now()
		}
		a.syncOutgoingTyping()
	}()

	if id != "<Tab>" {
		a.resetNickCompletion()
	}

	if a.jumpMode {
		if a.handleJumpKey(id) {
			return
		}
	}

	switch id {
	case "<M-l>", "<M-L>", "<A-l>", "<A-L>", "<Alt-l>", "<Alt-L>":
		a.setCopyMode(true)
	case "<C-c>":
		a.stopped.Store(true)
	case "<Enter>":
		line := strings.TrimSpace(a.input.Text)
		a.input.Text = ""
		a.input.Cursor = 0
		if line != "" {
			a.execute(line)
		}
	case "<Backspace>", "<C-h>":
		a.input.Backspace()
	case "<Left>":
		a.input.MoveCursorLeft()
	case "<Right>":
		a.input.MoveCursorRight()
	case "<Home>":
		a.input.Cursor = 0
	case "<End>":
		a.input.Cursor = len([]rune(a.input.Text))
		a.follow = true
		a.transcript.ScrollBottom()
	case "<C-u>":
		a.input.Text = ""
		a.input.Cursor = 0
	case "<C-n>":
		a.selectRelative(1)
	case "<F6>":
		a.jumpMode = true
		a.jumpDigits = ""
	case "<Tab>":
		a.completeNick()
	case "<C-p>":
		a.selectRelative(-1)
	case "<PageUp>":
		a.follow = false
		a.transcript.ScrollPageUp()
	case "<PageDown>":
		a.transcript.ScrollPageDown()
		a.resumeFollowAtBottom()
	case "<Up>":
		a.follow = false
		a.transcript.ScrollUp()
	case "<Down>":
		a.transcript.ScrollDown()
		a.resumeFollowAtBottom()
	case "<Space>":
		a.input.InsertRune(' ')
	default:
		if !strings.HasPrefix(id, "<") {
			for _, r := range id {
				a.input.InsertRune(r)
			}
		}
	}
}

// isCopyModeKey accepts the Meta/Alt spellings used by gotui/tcell and a few
// terminal backends. gotui normally reports Alt+L as <M-l>.
func isCopyModeKey(id string) bool {
	switch id {
	case "<M-l>", "<M-L>", "<A-l>", "<A-L>", "<Alt-l>", "<Alt-L>":
		return true
	default:
		return false
	}
}

// setCopyMode implements a WeeChat-style bare display. The normal widget
// layout is replaced by a borderless full-screen snapshot of the current
// transcript and terminal mouse reporting is disabled so the terminal can do
// native click-and-drag selection. While active, the main loop intentionally
// does not redraw the screen; network activity continues in the background.
func (a *App) setCopyMode(enabled bool) {
	if a.copyMode == enabled {
		return
	}
	a.copyMode = enabled
	a.resetNickCompletion()
	a.jumpMode = false
	a.jumpDigits = ""

	if enabled {
		if ui.DefaultBackend.Screen != nil {
			ui.DefaultBackend.Screen.DisableMouse()
		}
		ui.Clear()
		a.renderCopyMode()
		return
	}

	// gotui enables terminal mouse reporting during Init, so restore the same
	// backend state it had before entering bare mode. Copperline's general.mouse
	// setting still decides whether received mouse events are acted upon.
	if ui.DefaultBackend.Screen != nil {
		ui.DefaultBackend.Screen.EnableMouse()
	}
	ui.Clear()
	// Rebuild with clean gotui scroll state because the transcript geometry was
	// temporarily replaced by the full-screen bare view.
	a.transcriptReset = true
}

func (a *App) renderCopyMode() {
	w, h := ui.TerminalDimensions()
	if w < 1 || h < 1 {
		return
	}

	// Refresh the source rows once on entry/resize, then render an independent
	// widget so the normal transcript's scroll position and private topRow state
	// are untouched.
	a.rebuildCurrent()
	bare := widgets.NewList()
	bare.Border = false
	bare.WrapText = true
	bare.Rows = append([]string(nil), a.transcript.Rows...)
	bare.TextStyle = a.transcript.TextStyle
	bare.SelectedStyle = bare.TextStyle
	bare.SetRect(0, 0, w, h)
	if len(bare.Rows) > 0 {
		bare.SelectedRow = len(bare.Rows) - 1
		bare.ScrollBottom()
	}
	ui.Render(bare)
}

// syncOutgoingTyping translates edits in Copperline's input box into the
// IRCv3 +typing client tag. Active is refreshed every four seconds, paused is
// sent after four seconds without an edit, and Manager enforces the spec's
// three-second minimum interval between notifications for a target.
func (a *App) syncOutgoingTyping() {
	b := a.state.Current()
	server, target := "", ""
	if b != nil && b.Target != "" && b.Target != "*server*" {
		server, target = b.Server, b.Target
	}

	// Moving to another buffer ends the old target's typing state. If the
	// three-second throttle suppresses this done event, the peer will still
	// expire our previous active indication after six seconds.
	if server != a.typingServer || target != a.typingTarget {
		if a.typingServer != "" && a.typingTarget != "" && a.typingSentState != "" && a.typingSentState != "done" {
			_, _ = a.irc.SendTyping(a.typingServer, a.typingTarget, "done")
		}
		a.typingServer = server
		a.typingTarget = target
		a.typingSentState = ""
		a.typingLastSent = time.Time{}
	}
	if server == "" || target == "" {
		return
	}

	now := time.Now()
	text := strings.TrimSpace(a.input.Text)
	desired := "done"
	if text != "" && !strings.HasPrefix(text, "/") {
		if a.typingLastEdit.IsZero() {
			a.typingLastEdit = now
		}
		if now.Sub(a.typingLastEdit) >= 4*time.Second {
			desired = "paused"
		} else {
			desired = "active"
		}
	}

	if desired == "done" && (a.typingSentState == "" || a.typingSentState == "done") {
		return
	}
	shouldSend := desired != a.typingSentState
	if desired == "active" && a.typingSentState == "active" && now.Sub(a.typingLastSent) >= 4*time.Second {
		shouldSend = true
	}
	if !shouldSend {
		return
	}

	sent, err := a.irc.SendTyping(server, target, desired)
	if err != nil || !sent {
		// Connection loss and throttle suppression are both transient. The UI
		// ticker will retry without polluting the conversation with errors.
		return
	}
	a.typingSentState = desired
	a.typingLastSent = now
}

func typingInputTitle(nicks []string) string {
	switch len(nicks) {
	case 0:
		return "Message"
	case 1:
		return "Message — " + nicks[0] + " is typing…"
	case 2:
		return "Message — " + nicks[0] + " and " + nicks[1] + " are typing…"
	default:
		return fmt.Sprintf("Message — %s, %s +%d are typing…", nicks[0], nicks[1], len(nicks)-2)
	}
}

func (a *App) handleMouse(e ui.Event) {
	m, ok := mousePayload(e.Payload)
	if !ok {
		return
	}
	left := 28
	w, _ := ui.TerminalDimensions()
	if w < 70 {
		left = 20
	}

	switch e.ID {
	case "<MouseWheelUp>":
		if m.X >= left {
			a.follow = false
			a.transcript.ScrollAmount(-3)
		}
	case "<MouseWheelDown>":
		if m.X >= left {
			a.transcript.ScrollAmount(3)
			a.resumeFollowAtBottom()
		}
	case "<MouseLeft>":
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
		if len(a.userNicks) > 0 {
			_, h := ui.TerminalDimensions()
			rightStart := w - 22
			if w > 90 && m.X >= rightStart && m.Y > 0 && m.Y < h-1 {
				row := m.Y - 1
				if row >= 0 && row < len(a.userNicks) {
					if b := a.state.Current(); b != nil {
						nick := a.userNicks[row]
						if a.nickDoubleClicked(b.Server, nick) {
							a.state.Select(b.Server, nick)
							a.follow = true
						}
						return
					}
				}
			}
		}
		a.clearNickClick()
	}
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
		a.transcript.ScrollBottom()
	}
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
func (a *App) completeNick() bool {
	b := a.state.Current()
	if b == nil || !model.IsChannel(b.Target) {
		a.resetNickCompletion()
		return false
	}

	text := []rune(a.input.Text)
	cursor := a.input.Cursor
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(text) {
		cursor = len(text)
	}

	// A second (or later) Tab cycles the matches generated by the first Tab.
	if len(a.nickCompletionMatches) > 0 &&
		a.nickCompletionStart >= 0 &&
		a.nickCompletionEnd >= a.nickCompletionStart &&
		a.nickCompletionEnd <= len(text) {
		a.nickCompletionIndex = (a.nickCompletionIndex + 1) % len(a.nickCompletionMatches)
		a.applyNickCompletion(text, a.nickCompletionMatches[a.nickCompletionIndex])
		return true
	}

	start := cursor
	for start > 0 && !isCompletionSeparator(text[start-1]) {
		start--
	}
	if start == cursor {
		return false
	}

	fragment := string(text[start:cursor])
	if fragment == "" {
		return false
	}

	end := cursor
	for end < len(text) && !isCompletionSeparator(text[end]) {
		end++
	}

	// Status prefixes are display metadata, not part of the nick itself. Accept
	// one if the user happened to type it, but complete to the real nickname.
	if len([]rune(fragment)) > 1 && strings.ContainsRune("~&@%+", []rune(fragment)[0]) {
		fragment = string([]rune(fragment)[1:])
		start++
	}
	if fragment == "" {
		return false
	}

	var matches []string
	needle := strings.ToLower(fragment)
	for _, nick := range a.irc.Names(b.Server, b.Target) {
		if strings.HasPrefix(strings.ToLower(nick), needle) {
			matches = append(matches, nick)
		}
	}
	if len(matches) == 0 {
		a.resetNickCompletion()
		return false
	}

	firstWord := start == 0
	if firstWord && end < len(text) && text[end] == ' ' {
		end++
	}

	a.nickCompletionMatches = matches
	a.nickCompletionIndex = 0
	a.nickCompletionStart = start
	a.nickCompletionEnd = end
	a.nickCompletionFirst = firstWord
	a.applyNickCompletion(text, matches[0])
	return true
}

func (a *App) applyNickCompletion(text []rune, nick string) {
	start := a.nickCompletionStart
	end := a.nickCompletionEnd
	if start < 0 || end < start || end > len(text) {
		a.resetNickCompletion()
		return
	}

	replacement := nick
	if a.nickCompletionFirst {
		replacement += ": "
	}
	repl := []rune(replacement)
	updated := make([]rune, 0, len(text)-(end-start)+len(repl))
	updated = append(updated, text[:start]...)
	updated = append(updated, repl...)
	updated = append(updated, text[end:]...)

	a.input.Text = string(updated)
	a.nickCompletionEnd = start + len(repl)
	a.input.Cursor = a.nickCompletionEnd
}

func (a *App) resetNickCompletion() {
	a.nickCompletionMatches = nil
	a.nickCompletionIndex = 0
	a.nickCompletionStart = -1
	a.nickCompletionEnd = -1
	a.nickCompletionFirst = false
}

func isCompletionSeparator(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r'
}

// handleJumpKey handles numbered buffer jumping after F6.
// Digits are collected until Enter. Escape cancels. Any unrelated key cancels
// jump mode and is then handled normally, preserving the existing bindings.
func (a *App) handleJumpKey(id string) bool {
	switch id {
	case "<F6>", "<Escape>":
		a.jumpMode = false
		a.jumpDigits = ""
		return true
	case "<Backspace>", "<C-h>":
		if len(a.jumpDigits) > 0 {
			a.jumpDigits = a.jumpDigits[:len(a.jumpDigits)-1]
		}
		return true
	case "<Enter>":
		digits := a.jumpDigits
		a.jumpMode = false
		a.jumpDigits = ""
		if digits == "" {
			return true
		}
		n, err := strconv.Atoi(digits)
		if err == nil && a.selectBufferNumber(n) {
			return true
		}
		if b := a.state.Current(); b != nil {
			a.local(b.Server, b.Target, model.KindError, "no buffer numbered "+digits)
		}
		return true
	}

	if len(id) == 1 && id[0] >= '0' && id[0] <= '9' {
		// Buffer numbering starts at 1, so ignore leading zeroes.
		if id == "0" && a.jumpDigits == "" {
			return true
		}
		if len(a.jumpDigits) < 6 {
			a.jumpDigits += id
		}
		return true
	}

	// Do not steal any established keybinding.
	a.jumpMode = false
	a.jumpDigits = ""
	return false
}

func (a *App) selectBufferNumber(n int) bool {
	keys, _ := a.sidebarOrder()
	if n < 1 || n > len(keys) {
		return false
	}
	a.state.SelectKey(keys[n-1])
	a.follow = true
	return true
}

func (a *App) selectRelative(delta int) {
	keys, current := a.sidebarOrder()
	if len(keys) == 0 {
		return
	}

	idx := 0
	for i, key := range keys {
		if key == current {
			idx = i
			break
		}
	}
	idx = (idx + delta + len(keys)) % len(keys)
	a.state.SelectKey(keys[idx])
	a.follow = true
}

// sidebarOrder returns buffer keys in the exact order presented in the sidebar:
// each server buffer first, followed by that server's channels and queries in
// their stable creation order. Keyboard buffer navigation must use this same
// ordering so Ctrl-N/Ctrl-P never appear to jump around randomly.
func (a *App) sidebarOrder() ([]string, string) {
	buffers, current := a.state.Snapshot()
	servers := a.irc.ServerNames()
	keys := make([]string, 0, len(buffers))

	for _, server := range servers {
		serverKey := model.Key(server, "*server*")
		for _, b := range buffers {
			if model.Key(b.Server, b.Target) == serverKey {
				keys = append(keys, serverKey)
				break
			}
		}
		for _, b := range buffers {
			if b.Server == server && b.Target != "*server*" {
				keys = append(keys, model.Key(b.Server, b.Target))
			}
		}
	}

	return keys, current
}

func (a *App) render() {
	w, h := ui.TerminalDimensions()
	if w < 40 || h < 10 {
		return
	}
	left := 28
	if w < 70 {
		left = 20
	}
	right := 0
	if w > 90 {
		right = 22
	}
	bottom := 4

	a.rebuildSidebar()
	a.rebuildCurrent()

	a.sidebar.SetRect(0, 0, left, h-1)
	a.topic.SetRect(left, 0, w-right, 3)
	a.transcript.SetRect(left, 3, w-right, h-bottom)
	a.input.SetRect(left, h-bottom, w, h-1)
	a.status.SetRect(0, h-1, w, h)

	items := []ui.Drawable{a.sidebar, a.topic, a.transcript, a.input, a.status}
	if right > 0 {
		a.users.SetRect(w-right, 0, w, h-bottom)
		items = append(items, a.users)
	}
	ui.Render(items...)
}

func (a *App) rebuildSidebar() {
	buffers, current := a.state.Snapshot()
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
		label := numberLabel(len(rows)+1) + styled("◆", a.cfg.Theme.Server) + " " + server
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

func (a *App) rebuildCurrent() {
	b := a.state.Current()
	if b == nil {
		a.transcript.Rows = []string{"No buffer selected"}
		a.transcriptKey = ""
		a.transcriptReset = false
		a.users.Rows = nil
		a.topic.Text = ""
		a.input.Title = "Message"
		a.status.Text = "Copperline"
		return
	}

	key := model.Key(b.Server, b.Target)
	bufferChanged := key != a.transcriptKey
	if bufferChanged || a.transcriptReset {
		oldSelected := a.transcript.SelectedRow
		preserveSelection := !bufferChanged && a.transcriptReset && !a.follow

		// gotui List has a private topRow field that ScrollBottom does not reset.
		// Reusing one List across buffers can therefore carry a large buffer's
		// scroll offset into a shorter buffer, making only its last row visible.
		// A fresh widget gives the newly selected/resized transcript clean scroll
		// state without reaching into gotui internals.
		a.transcript = a.newTranscriptList()
		a.transcriptKey = key
		a.transcriptReset = false
		if bufferChanged {
			a.follow = true
		}
		if preserveSelection {
			a.transcript.SelectedRow = oldSelected
		}
	}

	a.input.Title = typingInputTitle(a.irc.TypingUsers(b.Server, b.Target))
	rows := make([]string, 0, len(b.Messages))
	for _, msg := range b.Messages {
		rows = append(rows, a.theme.formatMessage(msg, a.cfg.General.Timestamp))
	}
	if len(rows) == 0 {
		rows = []string{"No messages yet."}
	}
	a.transcript.Rows = rows
	a.transcript.Title = b.Server + " / " + b.Target
	if a.transcript.SelectedRow >= len(rows) {
		a.transcript.SelectedRow = len(rows) - 1
	}
	if a.transcript.SelectedRow < 0 {
		a.transcript.SelectedRow = 0
	}
	if a.follow {
		a.transcript.ScrollBottom()
	}

	topic := ""
	if model.IsChannel(b.Target) {
		topic = a.irc.ChannelTopic(b.Server, b.Target)
	}
	a.topic.Text = topic

	a.userNicks = a.irc.Names(b.Server, b.Target)
	a.users.Rows = make([]string, 0, len(a.userNicks))
	for _, nick := range a.userNicks {
		displayNick := a.irc.NickPrefix(b.Server, b.Target, nick) + nick
		a.users.Rows = append(a.users.Rows, styled(displayNick, a.theme.nickColor(nick)))
	}

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
		a.status.Text = fmt.Sprintf(" Jump to buffer: %s  Enter select  Esc cancel ", digits)
		return
	}
	a.status.Text = fmt.Sprintf(" %s  %s%s  nick:%s  IRCv3:%d caps  DCC:%d  Ctrl-N/P buffers  F6 jump  PgUp/PgDn scroll  /help ", b.Server, b.Target, joinState, nick, caps, offers)
}

func (a *App) execute(line string) {
	b := a.state.Current()
	if b == nil {
		return
	}
	if !strings.HasPrefix(line, "/") {
		// Sending a message always resumes live-follow. This guarantees the
		// server's echo of our message is brought into view even if manual
		// scrolling previously left the transcript's follow flag stale.
		a.follow = true
		a.transcript.ScrollBottom()
		if err := a.irc.SendMessage(b.Server, b.Target, line); err != nil {
			a.local(b.Server, b.Target, model.KindError, err.Error())
		}
		return
	}

	cmdline := strings.TrimSpace(strings.TrimPrefix(line, "/"))
	cmd, rest := cutWord(cmdline)
	cmd = strings.ToLower(cmd)
	arg1, tail := cutWord(rest)

	switch cmd {
	case "help":
		a.local(b.Server, b.Target, model.KindSystem, "commands: /server /buffer /connect /disconnect /join /part /query /msg /me /notice /ctcp /nick /topic /whois /raw /history /markread /caps /dcc /gotify /lua /close /quit")
	case "server":
		if arg1 == "" {
			a.local(b.Server, b.Target, model.KindSystem, "servers: "+strings.Join(a.irc.ServerNames(), ", "))
			return
		}
		a.state.Select(arg1, "*server*")
		a.follow = true
	case "buffer", "buf":
		if arg1 == "" {
			a.local(b.Server, b.Target, model.KindError, "usage: /buffer number")
			return
		}
		n, err := strconv.Atoi(arg1)
		if err != nil || !a.selectBufferNumber(n) {
			a.local(b.Server, b.Target, model.KindError, "no buffer numbered "+arg1)
		}
	case "connect":
		name := arg1
		if name == "" {
			name = b.Server
		}
		if err := a.irc.ConnectServer(name); err != nil {
			a.local(b.Server, b.Target, model.KindError, err.Error())
		}
	case "disconnect":
		name := arg1
		if name == "" {
			name = b.Server
		}
		if err := a.irc.DisconnectServer(name, strings.TrimSpace(tail)); err != nil {
			a.local(b.Server, b.Target, model.KindError, err.Error())
		}
	case "join", "j":
		if arg1 == "" {
			a.local(b.Server, b.Target, model.KindError, "usage: /join #channel [key]")
			return
		}
		key, _ := cutWord(tail)
		a.state.Ensure(b.Server, arg1)
		a.state.Select(b.Server, arg1)
		a.local(b.Server, arg1, model.KindSystem, "joining "+arg1+"...")
		if err := a.irc.Join(b.Server, arg1, key); err != nil {
			a.local(b.Server, arg1, model.KindError, err.Error())
		}
	case "part", "leave":
		channel := arg1
		reason := tail
		if channel == "" {
			channel = b.Target
			reason = rest
		}
		if !model.IsChannel(channel) {
			a.local(b.Server, b.Target, model.KindError, "not a channel")
			return
		}
		if err := a.irc.Part(b.Server, channel, strings.TrimSpace(reason)); err != nil {
			a.local(b.Server, b.Target, model.KindError, err.Error())
		}
	case "query", "q":
		if arg1 != "" {
			a.state.Select(b.Server, arg1)
			a.follow = true
		}
	case "msg":
		if arg1 == "" || strings.TrimSpace(tail) == "" {
			a.local(b.Server, b.Target, model.KindError, "usage: /msg nick message")
			return
		}
		a.state.Ensure(b.Server, arg1)
		if err := a.irc.SendMessage(b.Server, arg1, strings.TrimSpace(tail)); err != nil {
			a.local(b.Server, arg1, model.KindError, err.Error())
		}
	case "me":
		if err := a.irc.SendAction(b.Server, b.Target, rest); err != nil {
			a.local(b.Server, b.Target, model.KindError, err.Error())
		}
	case "notice":
		if arg1 == "" || strings.TrimSpace(tail) == "" {
			a.local(b.Server, b.Target, model.KindError, "usage: /notice target message")
			return
		}
		if err := a.irc.Notice(b.Server, arg1, strings.TrimSpace(tail)); err != nil {
			a.local(b.Server, b.Target, model.KindError, err.Error())
		}
	case "ctcp":
		ctcpCommand, ctcpText := cutWord(tail)
		if arg1 == "" || ctcpCommand == "" {
			a.local(b.Server, b.Target, model.KindError, "usage: /ctcp nick command [text]")
			return
		}
		a.state.Ensure(b.Server, arg1)
		if err := a.irc.SendCTCP(b.Server, arg1, ctcpCommand, strings.TrimSpace(ctcpText)); err != nil {
			a.local(b.Server, arg1, model.KindError, err.Error())
		}
	case "nick":
		if arg1 != "" {
			if err := a.irc.Nick(b.Server, arg1); err != nil {
				a.local(b.Server, b.Target, model.KindError, err.Error())
			}
		}
	case "topic":
		if !model.IsChannel(b.Target) {
			a.local(b.Server, b.Target, model.KindError, "select a channel first")
			return
		}
		if err := a.irc.Topic(b.Server, b.Target, rest); err != nil {
			a.local(b.Server, b.Target, model.KindError, err.Error())
		}
	case "whois":
		if arg1 != "" {
			if err := a.irc.Whois(b.Server, arg1); err != nil {
				a.local(b.Server, b.Target, model.KindError, err.Error())
			}
		}
	case "raw", "quote":
		if err := a.irc.Raw(b.Server, rest); err != nil {
			a.local(b.Server, b.Target, model.KindError, err.Error())
		}
	case "history":
		limit := 50
		if arg1 != "" {
			if n, err := strconv.Atoi(arg1); err == nil && n > 0 {
				limit = n
			}
		}
		if err := a.irc.RequestHistory(b.Server, b.Target, limit); err != nil {
			a.local(b.Server, b.Target, model.KindError, err.Error())
		}
	case "markread":
		msgid := ""
		if len(b.Messages) > 0 {
			msgid = b.Messages[len(b.Messages)-1].Tags["msgid"]
		}
		if err := a.irc.MarkRead(b.Server, b.Target, msgid); err != nil {
			a.local(b.Server, b.Target, model.KindError, err.Error())
		}
	case "caps":
		caps := a.irc.Capabilities(b.Server)
		a.local(b.Server, b.Target, model.KindSystem, "negotiated IRCv3: "+strings.Join(caps, ", "))
	case "dcc":
		a.executeDCC(b, arg1, tail)
	case "gotify":
		a.executeGotify(b, arg1)
	case "lua":
		a.executeLua(b, arg1, tail)
	case "close":
		if b.Target != "*server*" {
			a.state.Close(b.Server, b.Target)
		}
	case "quit", "exit":
		a.irc.Stop(strings.TrimSpace(rest))
		a.stopped.Store(true)
	default:
		if a.scripts != nil {
			handled, err := a.scripts.Command(cmd, rest, scripting.Context{Server: b.Server, Target: b.Target, Nick: a.irc.CurrentNick(b.Server)})
			if err != nil {
				a.local(b.Server, b.Target, model.KindError, "Lua /"+cmd+": "+err.Error())
				return
			}
			if handled {
				return
			}
		}
		a.local(b.Server, b.Target, model.KindError, "unknown command /"+cmd+" — try /help")
	}
}

func (a *App) executeLua(b *model.Buffer, sub, tail string) {
	if a.scripts == nil {
		a.local(b.Server, b.Target, model.KindError, "Lua scripting is disabled")
		return
	}
	switch strings.ToLower(sub) {
	case "list":
		loaded := a.scripts.List()
		if len(loaded) == 0 {
			a.local(b.Server, b.Target, model.KindSystem, "Lua: no scripts loaded")
			return
		}
		a.local(b.Server, b.Target, model.KindSystem, "Lua scripts: "+strings.Join(loaded, ", "))
	case "reload":
		loaded, err := a.scripts.Reload()
		if err != nil {
			a.local(b.Server, b.Target, model.KindError, "Lua reload: "+err.Error())
			return
		}
		a.local(b.Server, b.Target, model.KindSystem, fmt.Sprintf("Lua: loaded %d script(s)", len(loaded)))
	case "eval":
		code := strings.TrimSpace(tail)
		if code == "" {
			a.local(b.Server, b.Target, model.KindError, "usage: /lua eval <code>")
			return
		}
		if err := a.scripts.Eval(code); err != nil {
			a.local(b.Server, b.Target, model.KindError, "Lua eval: "+err.Error())
		}
	default:
		a.local(b.Server, b.Target, model.KindSystem, "usage: /lua list | /lua reload | /lua eval <code>")
	}
}

func (a *App) executeGotify(b *model.Buffer, sub string) {
	switch strings.ToLower(sub) {
	case "status":
		if a.gotify != nil && a.gotify.Enabled() {
			a.local(b.Server, b.Target, model.KindSystem, "Gotify enabled")
		} else {
			a.local(b.Server, b.Target, model.KindSystem, "Gotify disabled")
		}
	case "test":
		if a.gotify == nil || !a.gotify.Enabled() {
			a.local(b.Server, b.Target, model.KindError, "Gotify is disabled in configuration")
			return
		}
		server, target := b.Server, b.Target
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(a.cfg.Gotify.TimeoutSeconds)*time.Second)
			defer cancel()
			if err := a.gotify.Test(ctx); err != nil {
				a.local(server, target, model.KindError, "Gotify test failed: "+err.Error())
				return
			}
			a.local(server, target, model.KindSystem, "Gotify test notification sent")
		}()
	default:
		a.local(b.Server, b.Target, model.KindSystem, "usage: /gotify status | /gotify test")
	}
}

func (a *App) executeDCC(b *model.Buffer, sub, rest string) {
	sub = strings.ToLower(sub)
	peer, path := cutWord(rest)
	switch sub {
	case "list":
		offers := a.irc.DCCOffers()
		if len(offers) == 0 {
			a.local(b.Server, b.Target, model.KindDCC, "no pending offers")
			return
		}
		for _, o := range offers {
			a.local(b.Server, b.Target, model.KindDCC, fmt.Sprintf("%s %s from %s on %s", o.Kind, o.Filename, o.Peer, o.Server))
		}
	case "accept":
		if peer == "" {
			a.local(b.Server, b.Target, model.KindError, "usage: /dcc accept nick")
			return
		}
		err := a.irc.DCCAccept(b.Server, peer, func(line string) {
			a.local(b.Server, peer, model.KindDCC, "<"+peer+"> "+line)
		})
		if err != nil {
			a.local(b.Server, b.Target, model.KindError, err.Error())
		}
	case "send":
		if peer == "" || strings.TrimSpace(path) == "" {
			a.local(b.Server, b.Target, model.KindError, "usage: /dcc send nick /path/to/file")
			return
		}
		if err := a.irc.DCCSend(b.Server, peer, config.ExpandPath(strings.TrimSpace(path))); err != nil {
			a.local(b.Server, b.Target, model.KindError, err.Error())
		}
	default:
		a.local(b.Server, b.Target, model.KindDCC, "usage: /dcc list | /dcc accept nick | /dcc send nick path")
	}
}

// containsNickMention reports whether text contains nick as an IRC nickname
// token rather than as a substring of a longer nickname. Matching is
// case-insensitive and permits ordinary punctuation around the nickname.
func containsNickMention(text, nick string) bool {
	textRunes := []rune(strings.ToLower(text))
	nickRunes := []rune(strings.ToLower(nick))
	if len(nickRunes) == 0 || len(textRunes) < len(nickRunes) {
		return false
	}

	for i := 0; i+len(nickRunes) <= len(textRunes); i++ {
		match := true
		for j := range nickRunes {
			if textRunes[i+j] != nickRunes[j] {
				match = false
				break
			}
		}
		if !match {
			continue
		}
		if i > 0 && isIRCNickRune(textRunes[i-1]) {
			continue
		}
		end := i + len(nickRunes)
		if end < len(textRunes) && isIRCNickRune(textRunes[end]) {
			continue
		}
		return true
	}
	return false
}

func isIRCNickRune(r rune) bool {
	if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
		return true
	}
	switch r {
	case '-', '_', '[', ']', '\\', '`', '^', '{', '}', '|':
		return true
	default:
		return false
	}
}

func cutWord(s string) (string, string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ""
	}
	if i := strings.IndexAny(s, " \t"); i >= 0 {
		return s[:i], strings.TrimSpace(s[i+1:])
	}
	return s, ""
}

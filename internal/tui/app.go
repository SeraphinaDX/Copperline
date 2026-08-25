package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"copperline/internal/config"
	gotifynotify "copperline/internal/gotify"
	ircclient "copperline/internal/irc"
	"copperline/internal/logging"
	"copperline/internal/model"
	"copperline/internal/scripting"

	"github.com/gdamore/tcell/v3"
	ui "github.com/metaspartan/gotui/v5"
	"github.com/metaspartan/gotui/v5/widgets"
)

type transcriptCache struct {
	rows        []string
	selectedRow int
	start       uint64
	total       uint64
	fromLog     bool
	backlogRows int
}

type App struct {
	cfg     *config.Config
	state   *model.State
	irc     *ircclient.Manager
	logger  *logging.Logger
	gotify  *gotifynotify.Notifier
	scripts *scripting.Engine
	theme   uiTheme
	stopped atomic.Bool
	redraw  chan struct{}

	sidebar    *widgets.List
	transcript *transcriptList
	users      *widgets.List
	topic      *widgets.Paragraph
	input      *widgets.Input
	status     *widgets.Paragraph

	sidebarKeys           []string
	userNicks             []string
	userScroll            int
	follow                bool
	transcriptKey         string
	transcriptReset       bool
	transcriptStart       uint64
	transcriptTotal       uint64
	transcriptFromLog     bool
	transcriptBacklogRows int
	transcriptCaches      map[string]transcriptCache

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

	connectionMu     sync.Mutex
	connectionSeen   map[string]bool
	reconnectPending map[string]bool
}

func New(cfg *config.Config) *App {
	a := &App{
		cfg:              cfg,
		state:            model.New(cfg.General.HistoryLines),
		logger:           logging.New(cfg.General.LoggingEnabled(), cfg.General.LogDir, cfg.General.Timestamp),
		gotify:           gotifynotify.New(cfg.Gotify),
		theme:            newUITheme(cfg.Theme),
		follow:           true,
		redraw:           make(chan struct{}, 1),
		transcriptCaches: make(map[string]transcriptCache),
		connectionSeen:   make(map[string]bool),
		reconnectPending: make(map[string]bool),
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
				b := a.state.CurrentInfo()
				if b == nil {
					return scripting.Context{}
				}
				return scripting.Context{Server: b.Server, Target: b.Target, Nick: a.irc.CurrentNick(b.Server)}
			},
			CurrentNick: a.irc.CurrentNick,
		})
	}
	// Raw IRC events drive small pieces of user-facing connection feedback as
	// well as optional Lua hooks, so keep the event sink installed even when
	// scripting is disabled.
	a.irc.SetEventSink(a.onIRCEvent)
	// Wake the UI after Manager has fully applied IRC state changes. Transcript
	// messages also wake immediately from onMessage after state.Add.
	a.irc.SetUpdateSink(a.requestRedraw)
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
	a.topic.WrapText = true
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

func (a *App) newTranscriptList() *transcriptList {
	list := widgets.NewList()
	list.Title = "Messages"
	list.WrapText = true
	list.BorderRounded = true
	a.theme.applyBlock(list)
	list.SelectedStyle = list.TextStyle
	return &transcriptList{List: list}
}

func (a *App) Run() error {
	if err := ui.Init(); err != nil {
		return err
	}
	defer ui.Close()

	if a.scripts != nil {
		defer a.scripts.Close()
		loaded, err := a.scripts.Reload()
		b := a.state.CurrentInfo()
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
	// Typing-state maintenance still needs a small timer, but terminal redraws
	// are event-driven. The old 100 ms full-render loop repeatedly copied and
	// reformatted scrollback even when nothing changed, which made busy/long
	// buffers feel inconsistently delayed.
	typingTicker := time.NewTicker(250 * time.Millisecond)
	defer typingTicker.Stop()

	for !a.stopped.Load() {
		select {
		case e := <-events:
			a.handleUIEvent(e)
			if !a.copyMode {
				a.render()
			}
		case <-a.redraw:
			if !a.copyMode {
				a.render()
			}
		case <-typingTicker.C:
			a.syncOutgoingTyping()
			if !a.copyMode {
				a.refreshTypingIndicator()
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
	// Wake the UI immediately after the message becomes visible in state. Do
	// this before disk logging, notifications, or script callbacks so those
	// side effects can never hold up the on-screen conversation.
	a.requestRedraw()
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
	a.handleConnectionFeedback(ev)

	if a.scripts != nil {
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
}

// handleConnectionFeedback surfaces automatic reconnects where the user is
// actually looking. The server buffer already keeps the detailed connection
// record; this adds one concise line to the currently selected buffer on the
// affected network. DISCONNECTED and CLOSED can both be emitted for one loss,
// so reconnectPending also acts as the de-duplication guard.
func (a *App) handleConnectionFeedback(ev ircclient.Event) {
	switch ev.Command {
	case ircclient.EventDisconnected, ircclient.EventClosed:
		// A deliberate /disconnect must not be described as an automatic
		// reconnect. It also cancels any pending reconnect notice.
		if !a.irc.WantsConnection(ev.Server) {
			a.connectionMu.Lock()
			a.reconnectPending[ev.Server] = false
			a.connectionMu.Unlock()
			return
		}

		a.connectionMu.Lock()
		seen := a.connectionSeen[ev.Server]
		pending := a.reconnectPending[ev.Server]
		if seen && !pending {
			a.reconnectPending[ev.Server] = true
		}
		a.connectionMu.Unlock()
		if seen && !pending {
			a.connectionLine(ev.Server, "Connection lost; reconnecting…")
		}

	case ircclient.EventConnected:
		a.connectionMu.Lock()
		wasReconnect := a.reconnectPending[ev.Server]
		a.connectionSeen[ev.Server] = true
		a.reconnectPending[ev.Server] = false
		a.connectionMu.Unlock()
		if wasReconnect {
			nick := a.irc.CurrentNick(ev.Server)
			if nick == "" {
				a.connectionLine(ev.Server, "Reconnected to "+ev.Server+".")
			} else {
				a.connectionLine(ev.Server, "Reconnected to "+ev.Server+" as "+nick+".")
			}
		}
	}
}

func (a *App) connectionLine(server, text string) {
	current := a.state.CurrentInfo()
	if current == nil || !strings.EqualFold(current.Server, server) {
		return
	}
	a.local(server, current.Target, model.KindSystem, text)
}

// requestRedraw coalesces background wake-ups without ever blocking an IRC
// callback. All actual widget mutation/rendering remains on the TUI goroutine.
func (a *App) requestRedraw() {
	select {
	case a.redraw <- struct{}{}:
	default:
	}
}

// refreshTypingIndicator is the only periodic visual maintenance Copperline
// needs. It repaints just the input widget when a typing state expires instead
// of rebuilding the entire interface on a timer.
func (a *App) refreshTypingIndicator() {
	b := a.state.CurrentInfo()
	title := "Message"
	if b != nil {
		title = typingInputTitle(a.irc.TypingUsers(b.Server, b.Target))
	}
	if title == a.input.Title {
		return
	}
	a.input.Title = title
	ui.Render(a.input)
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
	defer func() {
		if a.input.Text != beforeText {
			a.typingLastEdit = time.Now()
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
		// If follow_bottom is rebound, End retains its normal input-editing role.
		a.input.Cursor = len([]rune(a.input.Text))
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
	bare := &transcriptList{List: widgets.NewList()}
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
	b := a.state.CurrentInfo()
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
	if w <= 90 || amount == 0 {
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
	if w <= 90 {
		return false
	}
	return x >= w-userPaneWidth && x < w && y >= 0 && y < h-uiBottomRows
}

func (a *App) mouseOverUserRows(x, y int) bool {
	w, h := ui.TerminalDimensions()
	if w <= 90 {
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
func (a *App) completeNick() bool {
	b := a.state.CurrentInfo()
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

// handleJumpKey handles numbered buffer jumping after the configured jump key.
// Digits are collected until Enter. The configured cancel key aborts. Any
// unrelated key cancels jump mode and is then handled normally.
func (a *App) handleJumpKey(e ui.Event) bool {
	id := e.ID
	if keyBindingMatchesEvent(a.cfg.Keybindings.JumpBuffer, e) ||
		keyBindingMatchesEvent(a.cfg.Keybindings.JumpCancel, e) {
		a.jumpMode = false
		a.jumpDigits = ""
		return true
	}

	switch id {
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
		if b := a.state.CurrentInfo(); b != nil {
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
// ordering so next/previous-buffer bindings never appear to jump randomly.
func (a *App) sidebarOrder() ([]string, string) {
	buffers, current := a.state.SnapshotInfo()
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
		right = userPaneWidth
	}
	bottom := uiBottomRows

	a.rebuildSidebar()
	a.rebuildCurrent()

	a.sidebar.SetRect(0, 0, left, h-1)
	topicHeight := topicWidgetHeight(a.topic.Text, w-right-left)
	a.topic.SetRect(left, 0, w-right, topicHeight)
	a.transcript.SetRect(left, topicHeight, w-right, h-bottom)
	a.input.SetRect(left, h-bottom, w, h-1)
	a.status.SetRect(0, h-1, w, h)

	items := []ui.Drawable{a.sidebar, a.topic, a.transcript, a.input, a.status}
	if right > 0 {
		a.users.SetRect(w-right, 0, w, h-bottom)
		a.refreshUserRows(userListVisibleRows(h))
		items = append(items, a.users)
	}
	ui.Render(items...)
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
			if !a.restoreTranscriptCache(key) {
				a.transcriptStart = 0
				a.transcriptTotal = 0
				a.transcriptFromLog = false
				a.transcriptBacklogRows = 0

				// Seed a channel from disk only on its first visit. Later visits
				// restore the cached live transcript above.
				a.loadChannelLogBacklog(b)
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
	// TUI. In log-backlog mode transcriptTotal is initialized to the buffer's
	// current total, so old in-memory messages are intentionally skipped.
	window := a.state.CurrentWindow(a.transcriptTotal)
	if window == nil {
		return
	}

	a.input.Title = typingInputTitle(a.irc.TypingUsers(b.Server, b.Target))

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
	}

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
	a.status.Text = fmt.Sprintf(" %s  %s%s  nick:%s  IRCv3:%d caps  DCC:%d  %s/%s buffers  %s/%s users  %s jump  %s/%s scroll  /help ",
		b.Server, b.Target, joinState, nick, caps, offers,
		a.cfg.Keybindings.NextBuffer, a.cfg.Keybindings.PreviousBuffer,
		a.cfg.Keybindings.UserListDown, a.cfg.Keybindings.UserListUp,
		a.cfg.Keybindings.JumpBuffer,
		a.cfg.Keybindings.TranscriptPageUp, a.cfg.Keybindings.TranscriptPageDown)
}

// loadChannelLogBacklog initializes a newly selected channel from the last few
// plaintext log lines. Returning true means the caller should treat the
// transcript as disk-backed context plus only messages newer than b.Total.
func (a *App) loadChannelLogBacklog(b *model.BufferInfo) bool {
	if b == nil || !model.IsChannel(b.Target) || !a.cfg.General.LoggingEnabled() {
		return false
	}
	n := a.cfg.General.LogBacklogLinesValue()
	if n <= 0 {
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
	a.transcriptStart = b.Total
	a.transcriptTotal = b.Total
	return true
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
		a.scrollTranscriptBottom()
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
			delete(a.transcriptCaches, model.Key(b.Server, b.Target))
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

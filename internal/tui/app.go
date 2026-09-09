package tui

import (
	"fmt"
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

type inputHistoryState struct {
	entries []string
	index   int
	draft   string
	active  bool
}

type relayReconnectResult struct {
	backend ircclient.Backend
	err     error
}

type App struct {
	cfg     *config.Config
	state   *model.State
	irc     ircclient.Backend
	logger  *logging.Logger
	gotify  *gotifynotify.Notifier
	scripts *scripting.Engine
	theme   uiTheme
	stopped atomic.Bool
	redraw  chan struct{}

	relayReconnect     func() (ircclient.Backend, error)
	relayReconnectDone chan relayReconnectResult
	relayReconnecting  atomic.Bool
	typingInFlight     atomic.Bool

	sidebar      *widgets.List
	transcript   *transcriptList
	users        *widgets.List
	topic        *widgets.Paragraph
	input        *widgets.Input
	status       *widgets.Paragraph
	relayControl *widgets.Paragraph

	sidebarKeys           []string
	userNicks             []string
	userScroll            int
	follow                bool
	transcriptKey         string
	transcriptReset       bool
	forceScreenSync       bool
	channelListHidden     bool
	userListHidden        bool
	transcriptStart       uint64
	transcriptTotal       uint64
	transcriptFromLog     bool
	transcriptBacklogRows int
	transcriptCaches      map[string]transcriptCache
	inputHistory          map[string]*inputHistoryState

	nickCompletionMatches []string
	nickCompletionIndex   int
	nickCompletionStart   int
	nickCompletionEnd     int
	nickCompletionFirst   bool

	jumpMode   bool
	jumpDigits string
	search     searchState

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

	closedBuffersMu sync.RWMutex
	closedBuffers   map[string]bool

	startupComplete     atomic.Bool
	startupLastActivity atomic.Int64
}

func New(cfg *config.Config) *App {
	return NewWithBackend(cfg, ircclient.New(cfg, nil))
}

// NewWithBackend constructs the TUI around an IRC backend. Direct mode uses
// irc.Manager; relay-client mode supplies the SSH-backed relay client instead.
func NewWithBackend(cfg *config.Config, backend ircclient.Backend) *App {
	a := &App{
		cfg:                cfg,
		state:              model.New(cfg.General.HistoryLines),
		irc:                backend,
		logger:             logging.New(cfg.General.LoggingEnabled(), cfg.General.LogDir, cfg.General.Timestamp),
		gotify:             gotifynotify.New(cfg.Gotify),
		theme:              newUITheme(cfg.Theme),
		follow:             true,
		redraw:             make(chan struct{}, 1),
		relayReconnectDone: make(chan relayReconnectResult, 1),
		transcriptCaches:   make(map[string]transcriptCache),
		inputHistory:       make(map[string]*inputHistoryState),
		connectionSeen:     make(map[string]bool),
		reconnectPending:   make(map[string]bool),
		closedBuffers:      make(map[string]bool),
	}
	for _, s := range cfg.Servers {
		a.state.Ensure(s.Name, "*server*")
		for _, ch := range s.Channels {
			a.state.Ensure(s.Name, ch)
		}
	}
	// A relay client may intentionally have no local [[server]] entries. Seed
	// its sidebar from the relay's initial SSH snapshot instead.
	for _, server := range backend.ServerNames() {
		a.state.Ensure(server, "*server*")
		for _, target := range backend.KnownTargets(server) {
			a.state.Ensure(server, target)
		}
	}

	a.startupLastActivity.Store(time.Now().UnixNano())
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
			CurrentNick: func(server string) string {
				return a.irc.CurrentNick(server)
			},
		})
	}
	a.bindBackend(backend)
	a.makeWidgets()
	return a
}

func (a *App) bindBackend(backend ircclient.Backend) {
	backend.SetMessageSink(a.onMessage)
	backend.SetEventSink(a.onIRCEvent)
	backend.SetUpdateSink(a.onBackendUpdate)
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

	// Relay-client mode gets a dedicated reconnect control instead of hiding
	// the action inside the ordinary status text. Temporary status messages,
	// startup progress and long server/channel labels therefore cannot overwrite
	// or clip the user's escape hatch when the SSH attachment is unhealthy.
	a.relayControl = widgets.NewParagraph()
	a.relayControl.Border = false
	a.relayControl.WrapText = false
	a.theme.applyStatus(a.relayControl)
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
	defer func() { a.irc.Stop("Copperline exiting") }()
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
		case result := <-a.relayReconnectDone:
			a.finishRelayReconnect(result)
			if !a.copyMode {
				a.render()
			}
		case <-typingTicker.C:
			a.syncOutgoingTyping()
			if !a.copyMode {
				// During startup, repaint on the small existing ticker so the loading
				// indicator animates and can transition to ready after the initial
				// IRC event burst goes quiet. Once startup is complete, return to the
				// cheaper input-only typing-indicator refresh.
				wasStarting := !a.startupComplete.Load()
				loading, _ := a.startupLoading()
				if loading || wasStarting {
					a.render()
				} else {
					a.refreshTypingIndicator()
				}
			}
		}
	}
	return nil
}

func (a *App) isBufferClosed(server, target string) bool {
	if target == "" || target == "*server*" {
		return false
	}
	a.closedBuffersMu.RLock()
	closed := a.closedBuffers[model.Key(server, target)]
	a.closedBuffersMu.RUnlock()
	return closed
}

func (a *App) closeBufferLocally(server, target string) {
	if target == "" || target == "*server*" {
		return
	}
	a.closedBuffersMu.Lock()
	if a.closedBuffers == nil {
		a.closedBuffers = make(map[string]bool)
	}
	a.closedBuffers[model.Key(server, target)] = true
	a.closedBuffersMu.Unlock()
}

func (a *App) reopenBuffer(server, target string) {
	if target == "" || target == "*server*" {
		return
	}
	a.closedBuffersMu.Lock()
	delete(a.closedBuffers, model.Key(server, target))
	a.closedBuffersMu.Unlock()
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
	if msg.Replay {
		// A locally closed buffer must not be resurrected merely because a relay
		// attachment replays retained history for every known target. Live traffic
		// below is allowed to reopen it so genuinely new conversation is visible.
		if a.isBufferClosed(msg.Server, msg.Target) {
			return
		}
		// Relay-retained history is already persisted/announced by the relay
		// server. Display it as normal conversation text without generating
		// duplicate local logs, notifications, or Lua callbacks on each attach.
		// A force reconnect receives the relay's bounded history again, so skip
		// rows already retained locally while still accepting messages that
		// arrived during the disconnected gap.
		if !a.state.ContainsMessage(msg) {
			a.state.Add(msg)
			a.requestRedraw()
		}
		return
	}
	// Genuine new traffic reopens a locally hidden conversation. Routine relay
	// snapshots and replay history do not.
	a.reopenBuffer(msg.Server, msg.Target)

	// Establish the persistent-log boundary before this message becomes visible
	// to the UI. For dynamically joined channels, the first redraw can otherwise
	// race the first log write and classify current-session traffic as muted
	// backlog when the channel is opened or revisited later.
	_ = a.logger.BeginBuffer(msg.Server, msg.Target)
	a.state.Add(msg)
	// Wake the UI immediately after the message becomes visible in state. Do
	// this before the actual append, notifications, or script callbacks so those
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
	if !a.startupComplete.Load() {
		a.startupLastActivity.Store(time.Now().UnixNano())
	}
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

func (a *App) onBackendUpdate() {
	// Relay snapshots can introduce configured/dynamically joined buffers that
	// have not received a message yet. Keep the local navigation model aligned
	// with the backend before waking the UI.
	for _, server := range a.irc.ServerNames() {
		a.state.Ensure(server, "*server*")
		for _, target := range a.irc.KnownTargets(server) {
			if a.isBufferClosed(server, target) {
				continue
			}
			a.state.Ensure(server, target)
		}
	}
	a.requestRedraw()
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
	title, placeholder := a.inputDisplayState(b)
	if title == a.input.Title && placeholder == a.input.Placeholder {
		return
	}
	a.input.Title = title
	a.input.Placeholder = placeholder
	ui.Render(a.input)
}

type startupProgress struct {
	connectedServers int
	totalServers     int
	joinedChannels   int
	totalChannels    int
}

func (p startupProgress) allReady() bool {
	return p.connectedServers == p.totalServers && p.joinedChannels == p.totalChannels
}

func (a *App) currentStartupProgress() startupProgress {
	var p startupProgress
	if a.cfg.Relay.ModeValue() == "client" {
		for _, server := range a.irc.ServerNames() {
			if !a.irc.WantsConnection(server) {
				continue
			}
			p.totalServers++
			if a.irc.IsConnected(server) {
				p.connectedServers++
			}
		}
		return p
	}
	for _, server := range a.cfg.Servers {
		if !server.AutoConnect {
			continue
		}
		p.totalServers++
		if a.irc.IsConnected(server.Name) {
			p.connectedServers++
		}
		for _, channel := range server.Channels {
			p.totalChannels++
			if a.irc.IsJoined(server.Name, channel) {
				p.joinedChannels++
			}
		}
	}
	return p
}

// startupLoading remains true until every auto-connect server/channel is up
// and the initial burst of IRC state updates has been quiet briefly. The quiet
// period is important on large channels: JOIN may complete before NAMES/WHO
// synchronization has finished, which is exactly when the UI can still feel
// busy even though the socket is technically connected.
func (a *App) startupLoading() (bool, startupProgress) {
	p := a.currentStartupProgress()
	if a.startupComplete.Load() {
		return false, p
	}
	if p.totalServers == 0 {
		a.startupComplete.Store(true)
		return false, p
	}
	if !p.allReady() {
		return true, p
	}
	last := time.Unix(0, a.startupLastActivity.Load())
	if time.Since(last) < 750*time.Millisecond {
		return true, p
	}
	a.startupComplete.Store(true)
	return false, p
}

func startupSpinner(now time.Time) string {
	frames := [...]string{"|", "/", "-", "\\"}
	return frames[(now.UnixMilli()/250)%int64(len(frames))]
}

// inputDisplayState makes it explicit when the selected target cannot accept
// chat yet. Commands remain available during startup, but the normal Message
// title does not appear until the current server/channel is usable.
func (a *App) inputDisplayState(b *model.BufferInfo) (title, placeholder string) {
	if b == nil {
		return "Message", "Type a message or /help"
	}
	if a.cfg.Relay.ModeValue() == "client" && !a.relayTransportConnected() {
		if a.relayReconnecting.Load() {
			return "Relay reconnecting - please wait", "Your draft will be kept until the relay is available"
		}
		return "Relay disconnected", "Use " + a.cfg.Keybindings.RelayReconnect + " or click RECONNECT"
	}
	if !a.irc.IsConnected(b.Server) {
		if a.irc.WantsConnection(b.Server) {
			return "Connecting - please wait", "Waiting for " + b.Server + "..."
		}
		return "Disconnected", "Use /connect to connect to " + b.Server
	}
	if model.IsChannel(b.Target) && !a.irc.IsJoined(b.Server, b.Target) {
		return "Joining - please wait", "Waiting to join " + b.Target + "..."
	}
	return typingInputTitle(a.irc.TypingUsers(b.Server, b.Target)), "Type a message or /help"
}

// chatWaitReason is checked before clearing the input on Enter. Keeping the
// draft in place makes an early keypress harmless instead of making the user
// retype a message after startup finishes.
func (a *App) chatWaitReason(server, target string) string {
	if a.cfg.Relay.ModeValue() == "client" && !a.relayTransportConnected() {
		if a.relayReconnecting.Load() {
			return "Relay SSH connection is being re-established; message kept in input."
		}
		return "Relay SSH connection is unavailable; message kept in input. Use " + a.cfg.Keybindings.RelayReconnect + " or click RECONNECT."
	}
	if !a.irc.IsConnected(server) {
		if a.irc.WantsConnection(server) {
			return "Still connecting to " + server + "; message kept in input."
		}
		return "Not connected to " + server + "; message kept in input."
	}
	if model.IsChannel(target) && !a.irc.IsJoined(server, target) {
		return "Still joining " + target + "; message kept in input."
	}
	return ""
}

func (a *App) notifyGotify(msg model.Message, self string) {
	if !a.gotify.Enabled() {
		return
	}
	if self == "" {
		self = a.irc.CurrentNick(msg.Server)
	}
	a.gotify.NotifyMessage(a.cfg.Gotify, msg, self)
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
			a.queueTyping(a.typingServer, a.typingTarget, "done")
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

	if !a.queueTyping(server, target, desired) {
		return
	}
	a.typingSentState = desired
	a.typingLastSent = now
}

// Typing is advisory. Keep at most one request in flight and drop intermediate
// states instead of making keystrokes wait for a relay response. Active states
// refresh periodically; paused/done indicators also expire at the receiver.
func (a *App) queueTyping(server, target, state string) bool {
	if !a.cfg.General.SendTypingEnabled() || a.relayReconnecting.Load() ||
		!a.relayTransportConnected() || !a.irc.IsConnected(server) {
		return false
	}
	if !a.typingInFlight.CompareAndSwap(false, true) {
		return false
	}
	backend := a.irc
	go func() {
		defer a.typingInFlight.Store(false)
		_, _ = backend.SendTyping(server, target, state)
	}()
	return true
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

func containsNickMention(text, nick string) bool { return model.ContainsNickMention(text, nick) }

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

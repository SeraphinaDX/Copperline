package tui

import (
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"copperline/internal/config"
	ircclient "copperline/internal/irc"
	"copperline/internal/logging"
	"copperline/internal/model"

	ui "github.com/metaspartan/gotui/v5"
	"github.com/metaspartan/gotui/v5/widgets"
)

type App struct {
	cfg     *config.Config
	state   *model.State
	irc     *ircclient.Manager
	logger  *logging.Logger
	theme   uiTheme
	stopped atomic.Bool

	sidebar    *widgets.List
	transcript *widgets.List
	users      *widgets.List
	topic      *widgets.Paragraph
	input      *widgets.Input
	status     *widgets.Paragraph

	sidebarKeys []string
	userNicks   []string
	follow      bool
}

func New(cfg *config.Config) *App {
	a := &App{
		cfg:    cfg,
		state:  model.New(cfg.General.HistoryLines),
		logger: logging.New(cfg.General.LoggingEnabled(), cfg.General.LogDir, cfg.General.Timestamp),
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
	a.makeWidgets()
	return a
}

func (a *App) makeWidgets() {
	a.sidebar = widgets.NewList()
	a.sidebar.Title = "Copperline"
	a.sidebar.WrapText = false
	a.sidebar.BorderRounded = true
	a.theme.applyBlock(a.sidebar)

	a.transcript = widgets.NewList()
	a.transcript.Title = "Messages"
	a.transcript.WrapText = true
	a.transcript.BorderRounded = true
	a.theme.applyBlock(a.transcript)
	a.transcript.SelectedStyle = a.transcript.TextStyle

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

func (a *App) Run() error {
	if err := ui.Init(); err != nil {
		return err
	}
	defer ui.Close()
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
			a.render()
		case <-ticker.C:
			a.render()
		}
	}
	return nil
}

func (a *App) onMessage(msg model.Message) {
	a.state.Add(msg)
	_ = a.logger.Write(msg)
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
	switch e.Type {
	case ui.ResizeEvent:
		return
	case ui.MouseEvent:
		if a.cfg.General.Mouse {
			a.handleMouse(e)
		}
		return
	case ui.KeyboardEvent:
		a.handleKey(e.ID)
	}
}

func (a *App) handleKey(id string) {
	switch id {
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
	case "<C-n>", "<Tab>":
		a.selectRelative(1)
	case "<C-p>":
		a.selectRelative(-1)
	case "<PageUp>":
		a.follow = false
		a.transcript.ScrollPageUp()
	case "<PageDown>":
		a.transcript.ScrollPageDown()
	case "<Up>":
		a.follow = false
		a.transcript.ScrollUp()
	case "<Down>":
		a.transcript.ScrollDown()
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
	case "MouseWheelUp":
		if m.X >= left {
			a.follow = false
			a.transcript.ScrollAmount(-3)
		}
	case "MouseWheelDown":
		if m.X >= left {
			a.transcript.ScrollAmount(3)
		}
	case "MouseLeft":
		if m.X < left && m.Y > 0 {
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
						a.state.Select(b.Server, a.userNicks[row])
						a.follow = true
					}
				}
			}
		}
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
// ordering so Tab/Ctrl-N/Ctrl-P never appear to jump around randomly.
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
	for _, server := range servers {
		serverKey := model.Key(server, "*server*")
		label := styled("◆", a.cfg.Theme.Server) + " " + server
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
			label := prefix + b.Target
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
		a.users.Rows = nil
		a.topic.Text = ""
		a.status.Text = "Copperline"
		return
	}
	rows := make([]string, 0, len(b.Messages))
	for _, msg := range b.Messages {
		rows = append(rows, a.theme.formatMessage(msg, a.cfg.General.Timestamp))
	}
	if len(rows) == 0 {
		rows = []string{"No messages yet."}
	}
	a.transcript.Rows = rows
	a.transcript.Title = b.Server + " / " + b.Target
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
		a.users.Rows = append(a.users.Rows, styled(nick, a.theme.nickColor(nick)))
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
	a.status.Text = fmt.Sprintf(" %s  %s%s  nick:%s  IRCv3:%d caps  DCC:%d  Ctrl-N/P buffers  PgUp/PgDn scroll  /help ", b.Server, b.Target, joinState, nick, caps, offers)
}

func (a *App) execute(line string) {
	b := a.state.Current()
	if b == nil {
		return
	}
	if !strings.HasPrefix(line, "/") {
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
		a.local(b.Server, b.Target, model.KindSystem, "commands: /server /connect /disconnect /join /part /query /msg /me /notice /nick /topic /whois /raw /history /markread /caps /dcc /close /quit")
	case "server":
		if arg1 == "" {
			a.local(b.Server, b.Target, model.KindSystem, "servers: "+strings.Join(a.irc.ServerNames(), ", "))
			return
		}
		a.state.Select(arg1, "*server*")
		a.follow = true
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
	case "close":
		if b.Target != "*server*" {
			a.state.Close(b.Server, b.Target)
		}
	case "quit", "exit":
		a.irc.Stop(strings.TrimSpace(rest))
		a.stopped.Store(true)
	default:
		a.local(b.Server, b.Target, model.KindError, "unknown command /"+cmd+" — try /help")
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

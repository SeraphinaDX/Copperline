package gui

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"copperline/internal/clientcmd"
	"copperline/internal/config"
	"copperline/internal/irc"
	"copperline/internal/model"
	"copperline/internal/relay"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"golang.org/x/crypto/ssh"
)

// App's widgets and connection ownership live on Fyne's event goroutine.
// Only the thread-safe model, generation, and dirty flag cross that boundary.
type App struct {
	app                  fyne.App
	window               fyne.Window
	storage              Storage
	signer               ssh.Signer
	cfg                  *config.Config
	client               *relay.Client
	cancel               context.CancelFunc
	generation           atomic.Uint64
	dirty                atomic.Bool
	closed               atomic.Bool
	messageMu            sync.Mutex
	state                *model.State
	drafts               map[string]string
	clearThrough         map[string]uint64
	buffers              []model.BufferInfo
	users                []string
	configText           string
	title, status        *widget.Label
	topic, entry         *widget.Entry
	send, topicSet       *widget.Button
	channels, nicklist   *widget.List
	transcript           *widget.RichText
	scroll               *container.Scroll
	chat                 fyne.CanvasObject
	root                 *fyne.Container
	lastKey              string
	lastTotal            uint64
	sending              bool
	foreground           bool
	resume               bool
	done                 chan struct{}
	profiles             map[string]*profile
	profileID            string
	mobile               bool
	channelPane, userPane fyne.CanvasObject
	topicKey             string
	topicDirty           bool
	topicSyncing         bool
	topicSending         bool
	topicPending         bool
	topicPendingValue    string
	topicPendingAt       time.Time
}

type profile struct {
	state        *model.State
	drafts       map[string]string
	clearThrough map[string]uint64
}

func New(a fyne.App, configPath string) (*App, error) {
	storage := Storage{App: a, ConfigPath: config.ExpandPath(configPath)}
	text, err := storage.LoadConfig()
	if err != nil {
		return nil, err
	}
	signer, err := storage.Signer()
	if err != nil {
		return nil, fmt.Errorf("relay identity: %w", err)
	}
	mobile := fyne.CurrentDevice().IsMobile()
	g := &App{app: a, window: a.NewWindow("Copperline GUI"), storage: storage, signer: signer,
		configText: text, state: model.New(1000), drafts: map[string]string{}, clearThrough: map[string]uint64{}, foreground: true, done: make(chan struct{}), mobile: mobile}
	t := config.DefaultTheme()
	if cfg, err := config.Decode(text); err == nil {
		t = cfg.Theme
	}
	a.Settings().SetTheme(copperTheme{cfg: t, compact: !mobile})
	g.build()
	if mobile {
		g.window.Resize(fyne.NewSize(360, 720))
	} else {
		g.window.Resize(fyne.NewSize(1100, 720))
	}
	g.window.SetOnClosed(g.shutdown)
	if mobile {
		a.Lifecycle().SetOnExitedForeground(func() {
			fyne.Do(func() {
				g.foreground = false
				g.resume = g.client != nil || g.cancel != nil
				g.detach()
				g.status.SetText("Suspended — relay remains online")
			})
		})
		a.Lifecycle().SetOnEnteredForeground(func() {
			fyne.Do(func() {
				g.foreground = true
				if g.resume {
					g.resume = false
					g.connect()
				}
			})
		})
	}
	if validateGUIConfig(text) != nil {
		g.settings()
	}
	return g, nil
}

// SetIcon applies the caller-provided packaged icon to both the Fyne app and
// its main window. The GUI executable embeds cmd/copperline-gui/Icon.png.
func (g *App) SetIcon(icon fyne.Resource) {
	if icon == nil {
		return
	}
	g.app.SetIcon(icon)
	g.window.SetIcon(icon)
}

func (g *App) Run() {
	g.app.Lifecycle().SetOnStarted(func() {
		fyne.Do(func() {
			if validateGUIConfig(g.configText) == nil {
				g.connect()
			}
		})
	})
	go func() {
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-g.done:
				return
			case <-ticker.C:
				if g.dirty.Swap(false) {
					fyne.Do(func() {
						if !g.closed.Load() {
							g.refresh()
						}
					})
				}
			}
		}
	}()
	g.window.ShowAndRun()
}

func (g *App) build() {
	g.title = widget.NewLabel("Choose a channel")
	g.title.TextStyle.Bold = true
	g.topic = widget.NewEntry()
	g.topic.SetPlaceHolder("Channel topic")
	g.topic.OnChanged = func(string) {
		if g.topicSyncing {
			return
		}
		g.topicDirty = true
		g.topicPending = false
	}
	g.topic.OnSubmitted = func(string) { g.submitTopic() }
	g.topicSet = widget.NewButton("Set", g.submitTopic)
	g.status = widget.NewLabel("Not connected")
	g.status.Wrapping = fyne.TextWrapWord
	g.entry = widget.NewEntry()
	g.entry.SetPlaceHolder("Message or /command…")
	g.entry.OnSubmitted = func(string) { g.submit() }
	g.send = widget.NewButton("Send", g.submit)
	g.transcript = widget.NewRichText()
	g.transcript.Wrapping = fyne.TextWrapWord
	g.scroll = container.NewVScroll(g.transcript)

	g.channels = widget.NewList(func() int { return len(g.buffers) }, func() fyne.CanvasObject { return widget.NewLabel("") }, func(id widget.ListItemID, o fyne.CanvasObject) {
		b := g.buffers[id]
		label := b.Server + " / " + b.Target
		if b.Target == "*server*" {
			label = b.Server + " / status"
		}
		if b.Unread > 0 {
			label += fmt.Sprintf(" (%d)", b.Unread)
		}
		o.(*widget.Label).SetText(label)
	})
	g.channels.OnSelected = func(id widget.ListItemID) {
		if id >= 0 && id < len(g.buffers) {
			b := g.buffers[id]
			g.selectBuffer(b.Server, b.Target)
		}
	}
	// Fyne maps SecondaryTappable to right-click on desktop and long-press on
	// mobile, so the same nick action menu works naturally on both platforms.
	g.nicklist = widget.NewList(
		func() int { return len(g.users) },
		func() fyne.CanvasObject { return newSecondaryLabel() },
		func(id widget.ListItemID, o fyne.CanvasObject) {
			row := o.(*secondaryLabel)
			row.SetText(g.users[id])
			nick := strings.TrimLeft(g.users[id], "~&@%+")
			row.onSecondary = func(ev *fyne.PointEvent) {
				g.showNickMenu(nick, ev.AbsolutePosition)
			}
		},
	)
	g.nicklist.OnSelected = func(id widget.ListItemID) {
		if id < 0 || id >= len(g.users) {
			return
		}
		b := g.state.CurrentInfo()
		if b == nil {
			return
		}
		nick := strings.TrimLeft(g.users[id], "~&@%+")
		g.selectBuffer(b.Server, nick)
	}

	channelHeading := widget.NewLabel("CHANNELS")
	channelHeading.TextStyle.Bold = true
	userHeading := widget.NewLabel("USERS")
	userHeading.TextStyle.Bold = true
	g.channelPane = container.NewBorder(channelHeading, nil, nil, nil, g.channels)
	g.userPane = container.NewBorder(userHeading, nil, nil, nil, g.nicklist)
	topicBar := container.NewBorder(nil, nil, nil, g.topicSet, g.topic)

	var tools fyne.CanvasObject
	if g.mobile {
		// Keep the existing touch-friendly Android controls exactly as before.
		tools = container.NewGridWithColumns(2,
			widget.NewButton("Channels", func() { g.panel("Channels", g.channels) }),
			widget.NewButton("Users", func() { g.panel("Users", g.nicklist) }),
			widget.NewButton("Reconnect", g.connect),
			widget.NewButton("Settings", g.settings),
		)
		g.chat = container.NewBorder(
			container.NewVBox(g.title, topicBar),
			container.NewVBox(g.status, container.NewBorder(nil, nil, nil, g.send, g.entry)),
			nil, nil, g.scroll,
		)
	} else {
		// Desktop keeps navigation visible, so the main chrome can stay quiet.
		reconnect := widget.NewButtonWithIcon("Reconnect", theme.ViewRefreshIcon(), g.connect)
		reconnect.Importance = widget.LowImportance
		settings := widget.NewButtonWithIcon("Settings", theme.SettingsIcon(), g.settings)
		settings.Importance = widget.LowImportance
		g.send.Importance = widget.HighImportance
		g.topicSet.Importance = widget.LowImportance

		desktopHeader := container.NewBorder(
			nil, nil,
			container.NewVBox(g.title, topicBar),
			container.NewHBox(reconnect, settings),
			nil,
		)
		composer := container.NewBorder(nil, nil, nil, g.send, g.entry)
		g.chat = container.NewBorder(desktopHeader, container.NewVBox(g.status, composer), nil, nil, g.scroll)

		// These only appear if a desktop window is narrowed enough to hide sidebars.
		channels := widget.NewButtonWithIcon("Channels", theme.MenuIcon(), func() { g.panel("Channels", g.channels) })
		channels.Importance = widget.LowImportance
		users := widget.NewButtonWithIcon("Users", theme.AccountIcon(), func() { g.panel("Users", g.nicklist) })
		users.Importance = widget.LowImportance
		tools = container.NewHBox(channels, users)
	}

	responsive := &responsiveLayout{chat: g.chat, channels: g.channelPane, users: g.userPane, mobile: g.mobile}
	g.root = fyne.NewContainerWithLayout(responsive, tools, g.chat, g.channelPane, g.userPane)
	g.root.Layout = responsive
	g.window.SetContent(g.root)
	g.entry.Disable()
	g.send.Disable()
	g.topic.Disable()
	g.topicSet.Disable()
}

func (g *App) setTopicText(text string) {
	g.topicSyncing = true
	g.topic.SetText(text)
	g.topicSyncing = false
}

func (g *App) syncTopic(b *model.Buffer, c *relay.Client) {
	if b == nil || !model.IsChannel(b.Target) {
		g.topicKey = ""
		g.topicDirty = false
		g.topicPending = false
		g.topicPendingValue = ""
		if g.topic.Text != "" {
			g.setTopicText("")
		}
		g.topic.Disable()
		g.topicSet.Disable()
		return
	}

	key := model.Key(b.Server, b.Target)
	remote := ""
	if c != nil {
		remote = c.ChannelTopic(b.Server, b.Target)
	}

	if key != g.topicKey {
		g.topicKey = key
		g.topicDirty = false
		g.topicPending = false
		g.topicPendingValue = ""
		g.setTopicText(remote)
	} else if c != nil {
		if g.topicPending {
			if remote == g.topicPendingValue || time.Since(g.topicPendingAt) > 5*time.Second {
				g.topicPending = false
				g.topicPendingValue = ""
				g.topicDirty = false
				g.setTopicText(remote)
			}
		} else if !g.topicDirty && remote != g.topic.Text {
			g.setTopicText(remote)
		}
	}

	if c != nil && c.TransportConnected() && !g.topicSending {
		g.topic.Enable()
		g.topicSet.Enable()
	} else {
		g.topic.Disable()
		g.topicSet.Disable()
	}
}

func (g *App) submitTopic() {
	if g.topicSending || g.client == nil || !g.client.TransportConnected() {
		return
	}
	b := g.state.CurrentInfo()
	if b == nil || !model.IsChannel(b.Target) {
		return
	}

	c, text := g.client, g.topic.Text
	server, target := b.Server, b.Target
	key, generation := model.Key(server, target), g.generation.Load()
	g.topicSending = true
	g.topic.Disable()
	g.topicSet.Disable()

	go func() {
		err := c.Topic(server, target, text)
		if g.closed.Load() {
			return
		}
		fyne.Do(func() {
			g.topicSending = false
			current := g.state.CurrentInfo()
			sameBuffer := generation == g.generation.Load() && current != nil && model.Key(current.Server, current.Target) == key
			if err != nil {
				if sameBuffer {
					g.topicPending = false
					g.topicDirty = true
				}
				dialog.ShowError(err, g.window)
			} else if sameBuffer {
				g.topicDirty = false
				g.topicPending = true
				g.topicPendingValue = text
				g.topicPendingAt = time.Now()
			}
			g.refresh()
		})
	}()
}

func (g *App) showNickMenu(nick string, pos fyne.Position) {
	b := g.state.CurrentInfo()
	if b == nil || nick == "" {
		return
	}
	server, target := b.Server, b.Target

	items := []*fyne.MenuItem{
		fyne.NewMenuItem("Open Query", func() { g.selectBuffer(server, nick) }),
		fyne.NewMenuItem("WHOIS", func() { g.runNickCommands(server, target, "/whois "+nick) }),
		fyne.NewMenuItem("Slap!", func() { g.runNickCommands(server, target, "/me slaps "+nick+" around a bit with a large trout") }),
		fyne.NewMenuItem("Copy Nick", func() { g.window.Clipboard().SetContent(nick) }),
	}
	if model.IsChannel(target) {
		items = append(items,
			fyne.NewMenuItemSeparator(),
			fyne.NewMenuItem("Op", func() { g.runNickCommands(server, target, "/raw MODE "+target+" +o "+nick) }),
			fyne.NewMenuItem("Deop", func() { g.runNickCommands(server, target, "/raw MODE "+target+" -o "+nick) }),
			fyne.NewMenuItem("Voice", func() { g.runNickCommands(server, target, "/raw MODE "+target+" +v "+nick) }),
			fyne.NewMenuItem("Devoice", func() { g.runNickCommands(server, target, "/raw MODE "+target+" -v "+nick) }),
			fyne.NewMenuItemSeparator(),
			fyne.NewMenuItem("Kick", func() { g.runNickCommands(server, target, "/raw KICK "+target+" "+nick) }),
			fyne.NewMenuItem("Kick + Ban", func() {
				g.runNickCommands(server, target,
					"/raw MODE "+target+" +b "+nick+"!*@*",
					"/raw KICK "+target+" "+nick,
				)
			}),
		)
	}
	widget.ShowPopUpMenuAtPosition(fyne.NewMenu(nick, items...), g.window.Canvas(), pos)
}

func (g *App) runNickCommands(server, target string, commands ...string) {
	c := g.client
	if c == nil || !c.TransportConnected() {
		dialog.ShowError(fmt.Errorf("relay is not connected"), g.window)
		return
	}
	generation := g.generation.Load()
	go func() {
		ctx := clientcmd.Context{Backend: c, Server: server, Target: target}
		for _, command := range commands {
			if err := clientcmd.Execute(ctx, command); err != nil {
				if !g.closed.Load() {
					fyne.Do(func() { dialog.ShowError(err, g.window) })
				}
				return
			}
		}
		if generation == g.generation.Load() {
			g.dirty.Store(true)
		}
	}()
}

func (g *App) panel(title string, list *widget.List) {
	// Use a second list: a canvas object must never belong to two containers.
	copyList := widget.NewList(list.Length, list.CreateItem, list.UpdateItem)
	var d *dialog.CustomDialog
	copyList.OnSelected = func(id widget.ListItemID) { list.OnSelected(id); d.Hide() }
	d = dialog.NewCustom(title, "Close", copyList, g.window)
	d.Resize(fyne.NewSize(320, 480))
	d.Show()
}

func (g *App) selectBuffer(server, target string) {
	if b := g.state.CurrentInfo(); b != nil {
		g.drafts[model.Key(b.Server, b.Target)] = g.entry.Text
	}
	g.state.Select(server, target)
	g.entry.SetText(g.drafts[model.Key(server, target)])
	g.lastKey = ""
	g.refresh()
}

func (g *App) detach() {
	g.generation.Add(1)
	if g.cancel != nil {
		g.cancel()
		g.cancel = nil
	}
	if g.client != nil {
		client := g.client
		g.client = nil
		go client.Stop("GUI detached")
	}
	g.entry.Disable()
	g.send.Disable()
	g.topic.Disable()
	g.topicSet.Disable()
}

func (g *App) connect() {
	if !g.foreground || g.closed.Load() {
		return
	}
	if err := validateGUIConfig(g.configText); err != nil {
		dialog.ShowError(err, g.window)
		g.settings()
		return
	}
	cfg, _ := config.Decode(g.configText)
	g.detach()
	id := cfg.Relay.Address + "\x00" + cfg.Relay.User + "\x00" + cfg.Relay.HostKeyFingerprint
	if g.profiles == nil {
		g.profiles = map[string]*profile{}
	}
	if g.profileID != id {
		if b := g.state.CurrentInfo(); b != nil {
			g.drafts[model.Key(b.Server, b.Target)] = g.entry.Text
		}
		if g.profileID != "" {
			g.profiles[g.profileID] = &profile{g.state, g.drafts, g.clearThrough}
		}
		p := g.profiles[id]
		if p == nil {
			p = &profile{model.New(cfg.General.HistoryLines), map[string]string{}, map[string]uint64{}}
			g.profiles[id] = p
		}
		g.messageMu.Lock()
		g.state = p.state
		g.messageMu.Unlock()
		g.drafts, g.clearThrough = p.drafts, p.clearThrough
		g.entry.SetText("")
		if b := g.state.CurrentInfo(); b != nil {
			g.entry.SetText(g.drafts[model.Key(b.Server, b.Target)])
		}
		g.profileID = id
		g.lastKey = ""
		g.transcript.Segments = nil
		g.transcript.Refresh()
		g.topicKey = ""
		g.topicDirty = false
		g.topicPending = false
		g.topicPendingValue = ""
		g.setTopicText("")
	}
	g.cfg = cfg
	g.app.Settings().SetTheme(copperTheme{cfg: cfg.Theme, compact: !g.mobile})
	g.messageMu.Lock()
	g.state.MaxLines = cfg.General.HistoryLines
	g.messageMu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	g.cancel = cancel
	generation := g.generation.Load()
	g.status.SetText("Connecting to " + cfg.Relay.Address + "…")
	go func() {
		c, err := relay.NewClientWithOptions(cfg, relay.ClientOptions{Signer: g.signer, Context: ctx})
		fyne.Do(func() {
			if generation != g.generation.Load() || g.closed.Load() {
				if c != nil {
					go c.Stop("stale attachment")
				}
				return
			}
			if err != nil {
				g.cancel = nil
				cancel()
				g.status.SetText("Connection failed")
				dialog.ShowError(err, g.window)
				return
			}
			g.client = c
			g.cancel = nil
			cancel()
			// Use goroutines for sink installation: draining replay can be substantial.
			go func() {
				c.SetUpdateSink(func() {
					if generation == g.generation.Load() {
						g.dirty.Store(true)
					}
				})
				c.SetEventSink(func(ev irc.Event) {
					if reply, ok := irc.ParseWhoisReply(ev); ok {
						g.messageMu.Lock()
						defer g.messageMu.Unlock()
						if generation != g.generation.Load() {
							return
						}
						g.state.Add(model.Message{Time: ev.Time, Server: ev.Server, Target: "*server*", Text: reply.Text, Kind: model.KindSystem, SuppressUnread: true})
						g.dirty.Store(true)
					}
				})
				c.SetMessageSink(func(msg model.Message) {
					g.messageMu.Lock()
					defer g.messageMu.Unlock()
					if generation != g.generation.Load() {
						return
					}
					if msg.Replay && g.state.ContainsMessage(msg) {
						return
					}
					if model.Highlight(msg, c.CurrentNick(msg.Server), cfg.General.HighlightWords) {
						msg.Mention = true
					}
					g.state.Add(msg)
					g.dirty.Store(true)
				})
				g.dirty.Store(true)
			}()
			g.entry.Enable()
			g.send.Enable()
			g.status.SetText("Connected to " + cfg.Relay.Address)
			g.refresh()
		})
	}()
}

func (g *App) refresh() {
	c := g.client
	if c != nil {
		for _, server := range c.ServerNames() {
			g.state.Ensure(server, "*server*")
			for _, target := range c.KnownTargets(server) {
				g.state.Ensure(server, target)
			}
		}
		if !c.TransportConnected() {
			g.entry.Disable()
			g.send.Disable()
			g.status.SetText("Relay disconnected — tap Reconnect")
		}
	}
	b := g.state.Current()
	if b != nil {
		key := model.Key(b.Server, b.Target)
		if key != g.lastKey || b.Total != g.lastTotal {
			atBottom := g.scroll.Offset.Y+g.scroll.Size().Height >= g.transcript.Size().Height-24
			segments := []widget.RichTextSegment{}
			start := b.Total - uint64(len(b.Messages))
			for i, msg := range b.Messages {
				if start+uint64(i) < g.clearThrough[key] {
					continue
				}
				color := theme.ColorNameForeground
				switch {
				case msg.Mention:
					color = "mention"
				case msg.Kind == model.KindAction:
					color = "action"
				case msg.Kind == model.KindSystem:
					color = "system"
				case msg.Kind == model.KindError:
					color = theme.ColorNameError
				case msg.Kind == model.KindNotice:
					color = "notice"
				}
				stamp := "15:04"
				if g.cfg != nil {
					stamp = g.cfg.General.Timestamp
				}
				segments = append(segments, &widget.TextSegment{Text: model.FormatMessage(msg, stamp), Style: widget.RichTextStyle{ColorName: color, SizeName: theme.SizeNameText}})
			}
			g.transcript.Segments = segments
			g.transcript.Refresh()
			if key != g.lastKey || atBottom {
				g.scroll.ScrollToBottom()
				g.state.MarkReadThrough(b.Server, b.Target, b.Total)
			}
			g.lastKey, g.lastTotal = key, b.Total
		}
		g.title.SetText(b.Server + " / " + b.Target)
		g.users = nil
		if c != nil {
			for _, nick := range c.Names(b.Server, b.Target) {
				g.users = append(g.users, c.NickPrefix(b.Server, b.Target, nick)+nick)
			}
		}
	}
	g.syncTopic(b, c)
	g.buffers, _ = g.state.SnapshotInfo()
	g.channels.Refresh()
	if b := g.state.CurrentInfo(); b != nil {
		for i, item := range g.buffers {
			if model.Key(item.Server, item.Target) == model.Key(b.Server, b.Target) {
				callback := g.channels.OnSelected
				g.channels.OnSelected = nil
				g.channels.Select(i)
				g.channels.OnSelected = callback
				break
			}
		}
	}
	g.nicklist.Refresh()
}

func (g *App) submit() {
	if g.sending || g.client == nil || !g.client.TransportConnected() || strings.TrimSpace(g.entry.Text) == "" {
		return
	}
	b := g.state.CurrentInfo()
	if b == nil {
		return
	}
	c, text, generation := g.client, g.entry.Text, g.generation.Load()
	g.sending = true
	g.send.Disable()
	// Clearing immediately allows another buffer to keep its own draft. On
	// failure restore the unsent text only when it cannot overwrite new typing.
	key := model.Key(b.Server, b.Target)
	drafts, profileID := g.drafts, g.profileID
	g.entry.SetText("")
	delete(g.drafts, key)
	g.scroll.ScrollToBottom()
	go func() {
		var openServer, openTarget string
		clear := false
		err := clientcmd.Execute(clientcmd.Context{Backend: c, Server: b.Server, Target: b.Target, Open: func(s, t string) { openServer, openTarget = s, t }, Clear: func() { clear = true }}, text)
		if g.closed.Load() {
			return
		}
		fyne.Do(func() {
			g.sending = false
			if g.client != nil && g.client.TransportConnected() {
				g.send.Enable()
			}
			if err != nil {
				// An uncertain send is retained for manual review, never retried.
				if !g.restoreDraft(profileID, drafts, key, text) {
					g.showUnsent(text)
				}
				dialog.ShowError(err, g.window)
			} else if generation == g.generation.Load() {
				if clear {
					g.clearThrough[key] = b.Total
					g.lastKey = ""
				}
				if openTarget != "" {
					g.selectBuffer(openServer, openTarget)
				}
			}
			g.refresh()
		})
	}()
}

// restoreDraft cannot put a failed send into a different relay profile or
// overwrite text typed while confirmation was pending.
func (g *App) restoreDraft(profileID string, drafts map[string]string, key, text string) bool {
	current := g.state.CurrentInfo()
	if profileID == g.profileID && current != nil && model.Key(current.Server, current.Target) == key {
		if g.entry.Text != "" {
			return false
		}
		g.entry.SetText(text)
		return true
	}
	if drafts[key] == "" {
		drafts[key] = text
		return true
	}
	return false
}

func (g *App) showUnsent(text string) {
	e := widget.NewMultiLineEntry()
	e.SetText(text)
	dialog.ShowCustom("Unsent message — copy before closing", "Close", e, g.window)
}

func (g *App) settings() {
	e := widget.NewMultiLineEntry()
	e.Wrapping = fyne.TextWrapWord
	e.SetMinRowsVisible(8)
	e.SetText(g.configText)
	pub := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(g.signer.PublicKey())))
	copyKey := widget.NewButton("Copy public key", func() { g.window.Clipboard().SetContent(pub) })
	guidance := widget.NewLabel("Enter relay details and fingerprint.\nCopy the public key to the relay's\nauthorized_keys file and restart it.")
	guidance.Wrapping = fyne.TextWrapOff
	var d *dialog.CustomDialog
	save := widget.NewButton("Save and connect", func() {
		if err := validateGUIConfig(e.Text); err != nil {
			dialog.ShowError(err, g.window)
			return
		}
		if err := g.storage.SaveConfig(e.Text); err != nil {
			dialog.ShowError(err, g.window)
			return
		}
		g.configText = e.Text
		d.Hide()
		g.connect()
	})
	d = dialog.NewCustom("Relay setup (TOML)", "Cancel", container.NewBorder(container.NewVBox(guidance, copyKey), save, nil, nil, e), g.window)
	d.Resize(fyne.NewSize(640, 560))
	d.Show()
}

func (g *App) shutdown() {
	if g.closed.Swap(true) {
		return
	}
	close(g.done)
	g.detach()
}

package irc

import (
	"crypto/tls"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"copperline/internal/config"
	"copperline/internal/dcc"
	"copperline/internal/model"

	"github.com/lrstanley/girc"
)

type Manager struct {
	cfg      *config.Config
	mu       sync.RWMutex
	sessions map[string]*Session
	emit     func(model.Message)
	dcc      *dcc.Manager
}

type Session struct {
	cfg     config.ServerConfig
	client  *girc.Client
	mu      sync.Mutex
	desired bool
	running bool
}

var defaultCaps = []string{
	"account-notify",
	"account-tag",
	"away-notify",
	"batch",
	"cap-notify",
	"chghost",
	"echo-message",
	"extended-join",
	"invite-notify",
	"labeled-response",
	"message-tags",
	"multi-prefix",
	"server-time",
	"setname",
	"standard-replies",
	"userhost-in-names",
	"draft/chathistory",
	"draft/event-playback",
	"draft/extended-monitor",
	"draft/message-redaction",
	"draft/multiline",
	"draft/no-implicit-names",
	"draft/read-marker",
}

func New(cfg *config.Config, emit func(model.Message)) *Manager {
	m := &Manager{cfg: cfg, sessions: make(map[string]*Session), emit: emit}
	m.dcc = dcc.New(
		config.ExpandPath(cfg.DCC.DownloadDir),
		cfg.DCC.ListenAddr,
		cfg.DCC.AdvertiseIP,
		func(e dcc.Event) {
			m.emitMessage(model.Message{Time: time.Now(), Server: e.Server, Target: e.Peer, Nick: "", Text: e.Text, Kind: model.KindDCC})
		},
	)
	for _, sc := range cfg.Servers {
		s := &Session{cfg: sc, desired: sc.AutoConnect}
		s.client = m.newClient(sc)
		m.sessions[sc.Name] = s
	}
	return m
}

func (m *Manager) newClient(sc config.ServerConfig) *girc.Client {
	caps := make(map[string][]string)
	for _, capName := range append(append([]string{}, defaultCaps...), sc.Caps...) {
		capName = strings.TrimSpace(capName)
		if capName != "" {
			caps[capName] = nil
		}
	}
	gc := girc.Config{
		Server:        sc.Host,
		Port:          sc.Port,
		Nick:          sc.Nick,
		User:          sc.User,
		Name:          sc.RealName,
		ServerPass:    config.Secret(sc.Password, sc.PasswordEnv),
		SSL:           sc.TLS,
		SupportedCaps: caps,
		Version:       "Copperline IRC client",
	}
	if sc.TLS {
		gc.TLSConfig = &tls.Config{ServerName: sc.Host, InsecureSkipVerify: sc.SkipVerify, MinVersion: tls.VersionTLS12} //nolint:gosec
	}
	switch strings.ToLower(sc.SASL.Mechanism) {
	case "plain":
		gc.SASL = &girc.SASLPlain{User: sc.SASL.Username, Pass: config.Secret(sc.SASL.Password, sc.SASL.PasswordEnv)}
	case "external":
		gc.SASL = &girc.SASLExternal{Identity: sc.SASL.Identity}
	}
	c := girc.New(gc)
	m.installHandlers(sc.Name, c)
	return c
}

func (m *Manager) installHandlers(server string, c *girc.Client) {
	c.Handlers.Add(girc.ALL_EVENTS, func(client *girc.Client, e girc.Event) {
		m.handleEvent(server, client, e)
	})
	if m.cfg.DCC.Enabled {
		c.CTCP.SetBg("DCC", func(_ *girc.Client, ce girc.CTCPEvent) {
			if ce.Source == nil {
				return
			}
			offer, err := m.dcc.ParseOffer(server, ce.Source.Name, ce.Text)
			if err != nil {
				m.emitMessage(model.Message{Time: time.Now(), Server: server, Target: ce.Source.Name, Kind: model.KindError, Text: "DCC: " + err.Error()})
				return
			}
			text := "incoming DCC " + string(offer.Kind)
			if offer.Filename != "" {
				text += fmt.Sprintf(" %s (%d bytes)", offer.Filename, offer.Size)
			}
			text += " — use /dcc accept " + ce.Source.Name
			m.emitMessage(model.Message{Time: time.Now(), Server: server, Target: ce.Source.Name, Kind: model.KindDCC, Text: text})
		})
	}
}

func (m *Manager) Start() {
	for _, name := range m.ServerNames() {
		m.mu.RLock()
		s := m.sessions[name]
		m.mu.RUnlock()
		if s != nil && s.cfg.AutoConnect {
			_ = m.ConnectServer(name)
		}
	}
}

func (m *Manager) Stop(reason string) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, s := range m.sessions {
		s.mu.Lock()
		s.desired = false
		s.mu.Unlock()
		if s.client.IsConnected() {
			s.client.Quit(reason)
		} else {
			s.client.Close()
		}
	}
}

func (m *Manager) ConnectServer(name string) error {
	s, err := m.session(name)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.desired = true
	if s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = true
	s.mu.Unlock()
	go m.connectionLoop(name, s)
	return nil
}

func (m *Manager) connectionLoop(name string, s *Session) {
	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()
	for {
		s.mu.Lock()
		desired := s.desired
		s.mu.Unlock()
		if !desired {
			return
		}
		m.serverLine(name, model.KindSystem, fmt.Sprintf("connecting to %s:%d", s.cfg.Host, s.cfg.Port))
		err := s.client.Connect()
		s.mu.Lock()
		desired = s.desired
		s.mu.Unlock()
		if !desired {
			return
		}
		if err != nil {
			m.serverLine(name, model.KindError, "connection error: "+err.Error())
		} else {
			m.serverLine(name, model.KindSystem, "disconnected")
		}
		time.Sleep(time.Duration(m.cfg.General.ReconnectSecs) * time.Second)
	}
}

func (m *Manager) DisconnectServer(name, reason string) error {
	s, err := m.session(name)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.desired = false
	s.mu.Unlock()
	if s.client.IsConnected() {
		s.client.Quit(reason)
	} else {
		s.client.Close()
	}
	return nil
}

func (m *Manager) SendMessage(server, target, text string) error {
	c, err := m.client(server)
	if err != nil {
		return err
	}
	if target == "" || target == "*server*" {
		return errors.New("select a channel or query first")
	}
	c.Cmd.Message(target, text)
	if !c.HasCapability("echo-message") {
		m.emitMessage(model.Message{Time: time.Now(), Server: server, Target: target, Nick: c.GetNick(), Text: text, Kind: model.KindMessage})
	}
	return nil
}

func (m *Manager) SendAction(server, target, text string) error {
	c, err := m.client(server)
	if err != nil {
		return err
	}
	c.Cmd.Action(target, text)
	if !c.HasCapability("echo-message") {
		m.emitMessage(model.Message{Time: time.Now(), Server: server, Target: target, Nick: c.GetNick(), Text: text, Kind: model.KindAction})
	}
	return nil
}

func (m *Manager) Notice(server, target, text string) error {
	c, err := m.client(server)
	if err != nil {
		return err
	}
	c.Cmd.Notice(target, text)
	m.emitMessage(model.Message{Time: time.Now(), Server: server, Target: target, Nick: c.GetNick(), Text: text, Kind: model.KindNotice})
	return nil
}

func (m *Manager) Join(server, channel, key string) error {
	c, err := m.client(server)
	if err != nil {
		return err
	}
	if key != "" {
		c.Cmd.JoinKey(channel, key)
	} else {
		c.Cmd.Join(channel)
	}
	return nil
}

func (m *Manager) Part(server, channel, reason string) error {
	c, err := m.client(server)
	if err != nil {
		return err
	}
	if reason != "" {
		c.Cmd.PartMessage(channel, reason)
	} else {
		c.Cmd.Part(channel)
	}
	return nil
}

func (m *Manager) Nick(server, nick string) error {
	c, err := m.client(server)
	if err != nil {
		return err
	}
	c.Cmd.Nick(nick)
	return nil
}

func (m *Manager) Topic(server, channel, text string) error {
	c, err := m.client(server)
	if err != nil {
		return err
	}
	c.Cmd.Topic(channel, text)
	return nil
}

func (m *Manager) Whois(server, nick string) error {
	c, err := m.client(server)
	if err != nil {
		return err
	}
	c.Cmd.Whois(nick)
	return nil
}

func (m *Manager) Raw(server, line string) error {
	c, err := m.client(server)
	if err != nil {
		return err
	}
	return c.Cmd.SendRaw(line)
}

func (m *Manager) RequestHistory(server, target string, limit int) error {
	c, err := m.client(server)
	if err != nil {
		return err
	}
	if !c.HasCapability("draft/chathistory") && !c.HasCapability("chathistory") {
		return errors.New("server did not negotiate CHATHISTORY")
	}
	if limit <= 0 {
		limit = 50
	}
	return c.Cmd.SendRawf("CHATHISTORY LATEST %s * %d", target, limit)
}

func (m *Manager) MarkRead(server, target, msgid string) error {
	c, err := m.client(server)
	if err != nil {
		return err
	}
	if msgid == "" {
		return errors.New("no msgid to mark read")
	}
	if c.HasCapability("draft/read-marker") || c.HasCapability("read-marker") {
		return c.Cmd.SendRawf("MARKREAD %s timestamp=%s", target, time.Now().UTC().Format(time.RFC3339Nano))
	}
	return nil
}

func (m *Manager) Names(server, channel string) []string {
	c, err := m.client(server)
	if err != nil || !model.IsChannel(channel) {
		return nil
	}
	ch := c.LookupChannel(channel)
	if ch == nil {
		return nil
	}
	out := append([]string(nil), ch.UserList...)
	sort.Strings(out)
	return out
}

func (m *Manager) ChannelTopic(server, channel string) string {
	c, err := m.client(server)
	if err != nil {
		return ""
	}
	if ch := c.LookupChannel(channel); ch != nil {
		return ch.Topic
	}
	return ""
}

func (m *Manager) IsJoined(server, channel string) bool {
	c, err := m.client(server)
	if err != nil || !model.IsChannel(channel) {
		return false
	}
	return c.LookupChannel(channel) != nil
}

func (m *Manager) ServerNames() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, 0, len(m.sessions))
	for name := range m.sessions {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func (m *Manager) CurrentNick(server string) string {
	c, err := m.client(server)
	if err != nil {
		return ""
	}
	return c.GetNick()
}

func (m *Manager) Capabilities(server string) []string {
	c, err := m.client(server)
	if err != nil {
		return nil
	}
	all := append([]string{}, defaultCaps...)
	if s, err := m.session(server); err == nil {
		all = append(all, s.cfg.Caps...)
	}
	seen := map[string]bool{}
	var out []string
	for _, capName := range all {
		if !seen[capName] && c.HasCapability(capName) {
			seen[capName] = true
			out = append(out, capName)
		}
	}
	sort.Strings(out)
	return out
}

func (m *Manager) DCCOffers() []dcc.Offer { return m.dcc.Offers() }

func (m *Manager) DCCAccept(server, peer string, onChatLine func(string)) error {
	offers := m.dcc.Offers()
	for _, o := range offers {
		if o.Server == server && strings.EqualFold(o.Peer, peer) {
			if o.Kind == dcc.Chat {
				return m.dcc.AcceptChat(server, peer, onChatLine)
			}
			return m.dcc.AcceptSend(server, peer)
		}
	}
	return errors.New("no pending DCC offer from that peer")
}

func (m *Manager) DCCSend(server, peer, path string) error {
	if !m.cfg.DCC.Enabled {
		return errors.New("DCC is disabled in configuration")
	}
	c, err := m.client(server)
	if err != nil {
		return err
	}
	return m.dcc.SendFile(server, peer, path, func(payload string) {
		c.Cmd.SendCTCP(peer, "DCC", payload)
	})
}

func (m *Manager) handleEvent(server string, c *girc.Client, e girc.Event) {
	when := e.Timestamp
	if when.IsZero() {
		when = time.Now()
	}
	tags := tagsMap(e.Tags)
	source := "server"
	if e.Source != nil && e.Source.Name != "" {
		source = e.Source.Name
	}

	switch e.Command {
	case girc.CONNECTED:
		m.serverLine(server, model.KindSystem, "connected as "+c.GetNick())
		if s, err := m.session(server); err == nil && len(s.cfg.Channels) > 0 {
			c.Cmd.Join(s.cfg.Channels...)
		}
		return
	case girc.DISCONNECTED, girc.CLOSED:
		return
	case girc.PRIVMSG:
		if ok, ctcp := e.IsCTCP(); ok {
			if ctcp != nil && strings.EqualFold(ctcp.Command, "DCC") {
				return
			}
			if ctcp != nil && !strings.EqualFold(ctcp.Command, "ACTION") {
				m.emitMessage(model.Message{Time: when, Server: server, Target: source, Nick: source, Text: "CTCP " + ctcp.Command + " " + ctcp.Text, Kind: model.KindSystem, Tags: tags})
				return
			}
		}
		if len(e.Params) < 2 {
			return
		}
		target := e.Params[0]
		if strings.EqualFold(target, c.GetNick()) {
			target = source
		}
		kind := model.KindMessage
		text := e.Last()
		if e.IsAction() {
			kind = model.KindAction
			text = e.StripAction()
		}
		m.emitMessage(model.Message{Time: when, Server: server, Target: target, Nick: source, Text: text, Kind: kind, Tags: tags})
		return
	case girc.NOTICE:
		if len(e.Params) < 2 {
			return
		}
		target := e.Params[0]
		if strings.EqualFold(target, c.GetNick()) {
			target = source
		}
		m.emitMessage(model.Message{Time: when, Server: server, Target: target, Nick: source, Text: e.Last(), Kind: model.KindNotice, Tags: tags})
		return
	case girc.JOIN:
		if len(e.Params) > 0 {
			m.emitMessage(model.Message{Time: when, Server: server, Target: e.Params[0], Kind: model.KindSystem, Text: source + " joined", Tags: tags})
		}
		return
	case girc.PART:
		if len(e.Params) > 0 {
			text := source + " left"
			if len(e.Params) > 1 {
				text += " (" + e.Last() + ")"
			}
			m.emitMessage(model.Message{Time: when, Server: server, Target: e.Params[0], Kind: model.KindSystem, Text: text, Tags: tags})
		}
		return
	case girc.KICK:
		if len(e.Params) >= 2 {
			m.emitMessage(model.Message{Time: when, Server: server, Target: e.Params[0], Kind: model.KindSystem, Text: fmt.Sprintf("%s kicked %s: %s", source, e.Params[1], e.Last()), Tags: tags})
		}
		return
	case girc.TOPIC:
		if len(e.Params) >= 2 {
			m.emitMessage(model.Message{Time: when, Server: server, Target: e.Params[0], Kind: model.KindSystem, Text: source + " changed topic: " + e.Last(), Tags: tags})
		}
		return
	case girc.NICK:
		m.serverLine(server, model.KindSystem, source+" is now known as "+e.Last())
		return
	case girc.QUIT:
		m.serverLine(server, model.KindSystem, source+" quit: "+e.Last())
		return
	case "FAIL", "WARN", "NOTE":
		m.serverLine(server, model.KindSystem, e.Command+": "+e.Last())
		return
	}

	if pretty, ok := e.Pretty(); ok && pretty != "" {
		m.emitMessage(model.Message{Time: when, Server: server, Target: "*server*", Kind: model.KindSystem, Text: pretty, Tags: tags})
		return
	}

	// girc intentionally hides many IRC numerics from Event.Pretty(). That is
	// useful for bots, but an interactive client must show failures such as
	// 473 (invite only), 475 (bad key), and 477 (registration required).
	if isNumericCommand(e.Command) {
		target := "*server*"
		if len(e.Params) > 1 && model.IsChannel(e.Params[1]) {
			target = e.Params[1]
		}
		text := e.Command
		if len(e.Params) > 0 {
			text += " " + strings.Join(e.Params, " ")
		}
		m.emitMessage(model.Message{Time: when, Server: server, Target: target, Kind: model.KindSystem, Text: text, Tags: tags})
	}
}

func isNumericCommand(command string) bool {
	if len(command) != 3 {
		return false
	}
	for _, r := range command {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func tagsMap(tags girc.Tags) map[string]string {
	if len(tags) == 0 {
		return nil
	}
	out := make(map[string]string, len(tags))
	for _, key := range tags.Keys() {
		if value, ok := tags.Get(key); ok {
			out[key] = value
		}
	}
	return out
}

func (m *Manager) serverLine(server string, kind model.Kind, text string) {
	m.emitMessage(model.Message{Time: time.Now(), Server: server, Target: "*server*", Kind: kind, Text: text})
}

func (m *Manager) emitMessage(msg model.Message) {
	if m.emit != nil {
		m.emit(msg)
	}
}

func (m *Manager) session(name string) (*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s := m.sessions[name]
	if s == nil {
		return nil, fmt.Errorf("unknown server %q", name)
	}
	return s, nil
}

func (m *Manager) client(name string) (*girc.Client, error) {
	s, err := m.session(name)
	if err != nil {
		return nil, err
	}
	if !s.client.IsConnected() {
		return nil, fmt.Errorf("server %q is not connected", name)
	}
	return s.client, nil
}

package relay

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"copperline/internal/config"
	"copperline/internal/dcc"
	ircclient "copperline/internal/irc"
	"copperline/internal/model"

	"golang.org/x/crypto/ssh"
)

const (
	relayRequestTimeout    = 10 * time.Second
	relayHeartbeatInterval = 5 * time.Second
)

type Client struct {
	cfg *config.Config

	sshClient *ssh.Client
	ch        ssh.Channel
	enc       *json.Encoder
	dec       *json.Decoder
	sendMu    sync.Mutex

	mu             sync.RWMutex
	snapshot       ircclient.StateSnapshot
	messageSink    func(model.Message)
	eventSink      func(ircclient.Event)
	updateSink     func()
	pendingHistory []model.Message
	dccCallbacks   map[string]func(string)
	connected      bool

	nextID    atomic.Uint64
	pendingMu sync.Mutex
	pending   map[uint64]chan frame
	done      chan struct{}
	closeOnce sync.Once
}

func NewClient(cfg *config.Config) (*Client, error) {
	signer, generatedKey, publicKeyPath, err := loadOrCreateClientSigner(cfg.Relay.PrivateKey, cfg.Relay.PrivateKeyPassphraseEnv)
	if err != nil {
		return nil, fmt.Errorf("relay private key: %w", err)
	}

	hostKeyCallback := func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		got := ssh.FingerprintSHA256(key)
		want := strings.TrimSpace(cfg.Relay.HostKeyFingerprint)
		if cfg.Relay.InsecureSkipHostKey {
			return nil
		}
		if want == "" {
			return fmt.Errorf("relay host key is %s; set [relay].host_key_fingerprint", got)
		}
		if got != want {
			return fmt.Errorf("relay host key mismatch: got %s, expected %s", got, want)
		}
		return nil
	}

	sshCfg := &ssh.ClientConfig{
		User:            cfg.Relay.User,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: hostKeyCallback,
		Timeout:         15 * time.Second,
		ClientVersion:   "SSH-2.0-Copperline-relay",
	}
	sshClient, err := ssh.Dial("tcp", cfg.Relay.Address, sshCfg)
	if err != nil {
		if generatedKey {
			return nil, fmt.Errorf("connect relay %s: %w (a new Copperline relay key was generated; add %s to the relay server authorized_keys file)", cfg.Relay.Address, err, publicKeyPath)
		}
		return nil, fmt.Errorf("connect relay %s: %w", cfg.Relay.Address, err)
	}
	ch, requests, err := sshClient.OpenChannel(channelType, nil)
	if err != nil {
		sshClient.Close()
		return nil, fmt.Errorf("open Copperline relay channel: %w", err)
	}
	go ssh.DiscardRequests(requests)

	c := &Client{
		cfg:          cfg,
		sshClient:    sshClient,
		ch:           ch,
		enc:          json.NewEncoder(ch),
		dec:          json.NewDecoder(ch),
		pending:      make(map[uint64]chan frame),
		done:         make(chan struct{}),
		dccCallbacks: make(map[string]func(string)),
		connected:    true,
	}

	if err := c.send(frame{Type: "hello", Version: protocolVersion, Bool: cfg.Gotify.Enabled}); err != nil {
		c.close()
		return nil, err
	}
	if err := c.initialSync(); err != nil {
		c.close()
		return nil, err
	}
	go c.reader()
	go c.heartbeat()
	return c, nil
}

func (c *Client) initialSync() error {
	gotHello := false
	for {
		var f frame
		if err := c.dec.Decode(&f); err != nil {
			return fmt.Errorf("relay handshake: %w", err)
		}
		switch f.Type {
		case "hello":
			if f.Version != protocolVersion {
				return fmt.Errorf("relay protocol mismatch: server=%d client=%d", f.Version, protocolVersion)
			}
			gotHello = true
		case "history":
			for i := range f.Messages {
				f.Messages[i].Replay = true
			}
			c.pendingHistory = append(c.pendingHistory, f.Messages...)
		case "snapshot":
			if f.Snapshot != nil {
				c.snapshot = cloneSnapshot(*f.Snapshot)
			}
		case "ready":
			if !gotHello {
				return errors.New("relay became ready before protocol hello")
			}
			return nil
		case "error":
			return errors.New(f.Error)
		}
	}
}

func (c *Client) reader() {
	defer c.close()
	for {
		var f frame
		if err := c.dec.Decode(&f); err != nil {
			return
		}
		switch f.Type {
		case "response":
			c.pendingMu.Lock()
			waiter := c.pending[f.ID]
			delete(c.pending, f.ID)
			c.pendingMu.Unlock()
			if waiter != nil {
				waiter <- f
			}
		case "message":
			if f.Message != nil {
				c.mu.RLock()
				sink := c.messageSink
				c.mu.RUnlock()
				if sink != nil {
					sink(*f.Message)
				}
			}
		case "event":
			if f.Event != nil {
				c.mu.RLock()
				sink := c.eventSink
				c.mu.RUnlock()
				if sink != nil {
					sink(*f.Event)
				}
			}
		case "snapshot":
			if f.Snapshot != nil {
				c.mu.Lock()
				c.snapshot = cloneSnapshot(*f.Snapshot)
				update := c.updateSink
				c.mu.Unlock()
				if update != nil {
					update()
				}
			}
		case "dcc_chat":
			key := dccCallbackKey(f.Server, f.Peer)
			c.mu.RLock()
			cb := c.dccCallbacks[key]
			c.mu.RUnlock()
			if cb != nil {
				cb(f.Text)
			}
		}
	}
}

func (c *Client) SetMessageSink(sink func(model.Message)) {
	c.mu.Lock()
	c.messageSink = sink
	history := append([]model.Message(nil), c.pendingHistory...)
	c.pendingHistory = nil
	c.mu.Unlock()
	if sink != nil {
		for _, msg := range history {
			sink(msg)
		}
	}
}

func (c *Client) SetEventSink(sink func(ircclient.Event)) {
	c.mu.Lock()
	c.eventSink = sink
	c.mu.Unlock()
}

func (c *Client) SetUpdateSink(sink func()) {
	c.mu.Lock()
	c.updateSink = sink
	c.mu.Unlock()
}

func (c *Client) Start() {}

// Stop detaches this Copperline client only. It deliberately does not stop the
// relay server's IRC sessions; that persistence is the purpose of relay mode.
func (c *Client) Stop(reason string) { c.close() }

// TransportConnected reports the health of the Copperline SSH attachment, not
// the cached IRC state carried by the last relay snapshot. The TUI uses this to
// avoid presenting a dead relay socket as a live IRC connection.
func (c *Client) TransportConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

func (c *Client) close() {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.connected = false
		update := c.updateSink
		c.mu.Unlock()
		close(c.done)
		_ = c.ch.Close()
		_ = c.sshClient.Close()

		c.pendingMu.Lock()
		for id, waiter := range c.pending {
			delete(c.pending, id)
			waiter <- frame{Type: "response", ID: id, Error: "relay connection closed"}
		}
		c.pendingMu.Unlock()
		if update != nil {
			update()
		}
	})
}

func (c *Client) send(f frame) error {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	return c.enc.Encode(f)
}

func (c *Client) call(action string, req frame) (frame, error) {
	select {
	case <-c.done:
		return frame{}, errors.New("relay connection is closed")
	default:
	}
	id := c.nextID.Add(1)
	req.Type = "request"
	req.ID = id
	req.Action = action
	waiter := make(chan frame, 1)
	c.pendingMu.Lock()
	c.pending[id] = waiter
	c.pendingMu.Unlock()
	if err := c.send(req); err != nil {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		// A write failure means this SSH attachment cannot safely be reused.
		// Closing it flips TransportConnected immediately and wakes the TUI.
		c.close()
		return frame{}, err
	}

	timer := time.NewTimer(relayRequestTimeout)
	defer timer.Stop()
	select {
	case resp := <-waiter:
		if resp.Error != "" {
			return resp, errors.New(resp.Error)
		}
		return resp, nil
	case <-timer.C:
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		// A relay request that cannot be acknowledged within the bounded timeout
		// leaves delivery uncertain. Tear down the attachment instead of keeping
		// stale cached IRC state marked as connected.
		c.close()
		return frame{}, fmt.Errorf("relay request %s timed out; SSH attachment closed", action)
	case <-c.done:
		return frame{}, errors.New("relay connection is closed")
	}
}

func (c *Client) heartbeat() {
	ticker := time.NewTicker(relayHeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			if _, err := c.call("ping", frame{}); err != nil {
				c.close()
				return
			}
		}
	}
}

func (c *Client) ConnectServer(name string) error {
	_, err := c.call("connect", frame{Server: name})
	return err
}
func (c *Client) DisconnectServer(name, reason string) error {
	_, err := c.call("disconnect", frame{Server: name, Text: reason})
	return err
}
func (c *Client) SendMessage(server, target, text string) error {
	_, err := c.call("message", frame{Server: server, Target: target, Text: text})
	return err
}
func (c *Client) SendTyping(server, target, state string) (bool, error) {
	resp, err := c.call("typing", frame{Server: server, Target: target, Text: state})
	return resp.Bool, err
}
func (c *Client) SendCTCP(server, target, command, text string) error {
	_, err := c.call("ctcp", frame{Server: server, Target: target, Extra: command, Text: text})
	return err
}
func (c *Client) SendAction(server, target, text string) error {
	_, err := c.call("action", frame{Server: server, Target: target, Text: text})
	return err
}
func (c *Client) Notice(server, target, text string) error {
	_, err := c.call("notice", frame{Server: server, Target: target, Text: text})
	return err
}
func (c *Client) Join(server, channel, key string) error {
	_, err := c.call("join", frame{Server: server, Target: channel, Extra: key})
	return err
}
func (c *Client) Part(server, channel, reason string) error {
	_, err := c.call("part", frame{Server: server, Target: channel, Text: reason})
	return err
}
func (c *Client) Nick(server, nick string) error {
	_, err := c.call("nick", frame{Server: server, Text: nick})
	return err
}
func (c *Client) Topic(server, channel, text string) error {
	_, err := c.call("topic", frame{Server: server, Target: channel, Text: text})
	return err
}
func (c *Client) Whois(server, nick string) error {
	_, err := c.call("whois", frame{Server: server, Target: nick})
	return err
}
func (c *Client) Raw(server, line string) error {
	_, err := c.call("raw", frame{Server: server, Text: line})
	return err
}
func (c *Client) RequestHistory(server, target string, limit int) error {
	_, err := c.call("history", frame{Server: server, Target: target, Limit: limit})
	return err
}
func (c *Client) MarkRead(server, target, msgid string) error {
	_, err := c.call("markread", frame{Server: server, Target: target, Text: msgid})
	return err
}

func (c *Client) snapshotCopy() ircclient.StateSnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return cloneSnapshot(c.snapshot)
}

func (c *Client) server(name string) (ircclient.ServerSnapshot, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, s := range c.snapshot.Servers {
		if strings.EqualFold(s.Name, name) {
			return cloneServerSnapshot(s), true
		}
	}
	return ircclient.ServerSnapshot{}, false
}

func targetSnapshot(s ircclient.ServerSnapshot, target string) (ircclient.TargetSnapshot, bool) {
	for _, t := range s.Targets {
		if strings.EqualFold(t.Target, target) {
			return t, true
		}
	}
	return ircclient.TargetSnapshot{}, false
}

func (c *Client) TypingUsers(server, target string) []string {
	s, ok := c.server(server)
	if !ok {
		return nil
	}
	t, ok := targetSnapshot(s, target)
	if !ok {
		return nil
	}
	return append([]string(nil), t.TypingUsers...)
}
func (c *Client) Names(server, channel string) []string {
	s, ok := c.server(server)
	if !ok {
		return nil
	}
	t, ok := targetSnapshot(s, channel)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(t.Users))
	for _, u := range t.Users {
		out = append(out, u.Nick)
	}
	return out
}
func (c *Client) NickPrefix(server, channel, nick string) string {
	s, ok := c.server(server)
	if !ok {
		return ""
	}
	t, ok := targetSnapshot(s, channel)
	if !ok {
		return ""
	}
	for _, u := range t.Users {
		if strings.EqualFold(u.Nick, nick) {
			return u.Prefix
		}
	}
	return ""
}
func (c *Client) ChannelTopic(server, channel string) string {
	s, ok := c.server(server)
	if !ok {
		return ""
	}
	t, ok := targetSnapshot(s, channel)
	if !ok {
		return ""
	}
	return t.Topic
}
func (c *Client) IsJoined(server, channel string) bool {
	s, ok := c.server(server)
	if !ok {
		return false
	}
	t, ok := targetSnapshot(s, channel)
	return ok && t.Joined
}
func (c *Client) ServerNames() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]string, 0, len(c.snapshot.Servers))
	for _, s := range c.snapshot.Servers {
		out = append(out, s.Name)
	}
	sort.Strings(out)
	return out
}
func (c *Client) KnownTargets(server string) []string {
	s, ok := c.server(server)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(s.Targets))
	for _, t := range s.Targets {
		out = append(out, t.Target)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i]) < strings.ToLower(out[j]) })
	return out
}

func (c *Client) CurrentNick(server string) string {
	s, ok := c.server(server)
	if !ok {
		return ""
	}
	return s.Nick
}
func (c *Client) IsConnected(server string) bool {
	c.mu.RLock()
	relayConnected := c.connected
	c.mu.RUnlock()
	if !relayConnected {
		return false
	}
	s, ok := c.server(server)
	return ok && s.Connected
}
func (c *Client) WantsConnection(server string) bool {
	s, ok := c.server(server)
	return ok && s.WantsConnection
}
func (c *Client) Capabilities(server string) []string {
	s, ok := c.server(server)
	if !ok {
		return nil
	}
	return append([]string(nil), s.Capabilities...)
}
func (c *Client) DCCOffers() []dcc.Offer {
	return c.snapshotCopy().DCCOffers
}
func (c *Client) DCCAccept(server, peer string, onChatLine func(string)) error {
	key := dccCallbackKey(server, peer)
	c.mu.Lock()
	c.dccCallbacks[key] = onChatLine
	c.mu.Unlock()
	_, err := c.call("dcc_accept", frame{Server: server, Peer: peer})
	if err != nil {
		c.mu.Lock()
		delete(c.dccCallbacks, key)
		c.mu.Unlock()
	}
	return err
}
func (c *Client) DCCSend(server, peer, path string) error {
	_, err := c.call("dcc_send", frame{Server: server, Peer: peer, Text: path})
	return err
}

func dccCallbackKey(server, peer string) string {
	return strings.ToLower(server) + "\x00" + strings.ToLower(peer)
}

func cloneSnapshot(in ircclient.StateSnapshot) ircclient.StateSnapshot {
	out := ircclient.StateSnapshot{DCCOffers: append([]dcc.Offer(nil), in.DCCOffers...)}
	out.Servers = make([]ircclient.ServerSnapshot, 0, len(in.Servers))
	for _, s := range in.Servers {
		out.Servers = append(out.Servers, cloneServerSnapshot(s))
	}
	return out
}

func cloneServerSnapshot(in ircclient.ServerSnapshot) ircclient.ServerSnapshot {
	out := in
	out.Capabilities = append([]string(nil), in.Capabilities...)
	out.Targets = make([]ircclient.TargetSnapshot, len(in.Targets))
	for i, t := range in.Targets {
		out.Targets[i] = t
		out.Targets[i].Users = append([]ircclient.UserSnapshot(nil), t.Users...)
		out.Targets[i].TypingUsers = append([]string(nil), t.TypingUsers...)
	}
	return out
}

var _ ircclient.Backend = (*Client)(nil)

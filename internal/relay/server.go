package relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"copperline/internal/config"
	"copperline/internal/gotify"
	ircclient "copperline/internal/irc"
	"copperline/internal/logging"
	"copperline/internal/model"

	"golang.org/x/crypto/ssh"
)

type Server struct {
	cfg         *config.Config
	irc         *ircclient.Manager
	state       *model.State
	logger      *logging.Logger
	sshConfig   *ssh.ServerConfig
	fingerprint string

	listener net.Listener
	peersMu  sync.Mutex
	peers    map[*serverPeer]struct{}
	nextPeer uint64
	notifier *gotify.Notifier

	// syncMu makes initial history attachment atomic with live-message delivery.
	syncMu sync.Mutex

	snapshotWake chan struct{}
	historyDirty atomic.Bool
}

type serverPeer struct {
	order         uint64
	notifications bool
	server        *Server
	ch            ssh.Channel
	transport     net.Conn
	enc           *json.Encoder
	dec           *json.Decoder
	sendQ         chan frame
	closed        chan struct{}
	closeOnce     sync.Once
}

func NewServer(cfg *config.Config) (*Server, error) {
	signer, err := loadOrCreateHostSigner(cfg.Relay.HostKey)
	if err != nil {
		return nil, fmt.Errorf("relay host key: %w", err)
	}
	if err := ensureAuthorizedKeysFile(cfg.Relay.AuthorizedKeys); err != nil {
		return nil, fmt.Errorf("relay authorized keys: %w", err)
	}

	sshCfg := &ssh.ServerConfig{
		NoClientAuth: false,
		MaxAuthTries: 6,
		PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if cfg.Relay.User != "" && conn.User() != cfg.Relay.User {
				return nil, fmt.Errorf("relay user %q is not allowed", conn.User())
			}
			authorized, err := loadAuthorizedKeys(cfg.Relay.AuthorizedKeys)
			if err != nil {
				return nil, fmt.Errorf("load relay authorized keys: %w", err)
			}
			if _, ok := authorized[string(key.Marshal())]; !ok {
				return nil, errors.New("relay public key is not authorized")
			}
			return &ssh.Permissions{Extensions: map[string]string{
				"pubkey-fingerprint": ssh.FingerprintSHA256(key),
			}}, nil
		},
	}
	sshCfg.AddHostKey(signer)

	s := &Server{
		cfg:          cfg,
		state:        model.New(cfg.General.HistoryLines),
		logger:       logging.New(cfg.General.LoggingEnabled(), cfg.General.LogDir, cfg.General.Timestamp),
		sshConfig:    sshCfg,
		fingerprint:  ssh.FingerprintSHA256(signer.PublicKey()),
		peers:        make(map[*serverPeer]struct{}),
		snapshotWake: make(chan struct{}, 1),
	}
	s.irc = ircclient.New(cfg, nil)
	if err := s.restoreHistory(); err != nil {
		return nil, fmt.Errorf("relay history: %w", err)
	}
	s.notifier = gotify.New(cfg.Gotify)
	s.irc.SetMessageSink(s.onMessage)
	s.irc.SetEventSink(s.onEvent)
	s.irc.SetUpdateSink(s.requestSnapshot)
	return s, nil
}

func (s *Server) HostKeyFingerprint() string { return s.fingerprint }

func (s *Server) Run(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.cfg.Relay.Listen)
	if err != nil {
		return fmt.Errorf("relay listen %s: %w", s.cfg.Relay.Listen, err)
	}
	s.listener = ln

	s.irc.Start()
	historyCtx, stopHistory := context.WithCancel(context.Background())
	historyDone := make(chan struct{})
	go func() { defer close(historyDone); s.historyLoop(historyCtx) }()
	defer func() {
		s.irc.Stop("Copperline relay stopping")
		stopHistory()
		<-historyDone
		if err := s.saveHistory(); err != nil {
			log.Printf("save relay history: %v", err)
		}
	}()
	defer ln.Close()

	go s.snapshotLoop(ctx)
	go func() {
		<-ctx.Done()
		_ = ln.Close()
		s.closePeers()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		go s.serveConn(conn)
	}
}

func (s *Server) serveConn(conn net.Conn) {
	// Do not let an unauthenticated TCP peer hold a relay goroutine forever by
	// stalling the SSH handshake. Once SSH authentication succeeds, normal SSH
	// keepalive/transport behavior takes over and the deadline is removed.
	_ = conn.SetDeadline(time.Now().Add(20 * time.Second))
	sshConn, chans, reqs, err := ssh.NewServerConn(conn, s.sshConfig)
	if err != nil {
		_ = conn.Close()
		return
	}
	_ = conn.SetDeadline(time.Time{})
	defer sshConn.Close()
	go ssh.DiscardRequests(reqs)

	for newChannel := range chans {
		if newChannel.ChannelType() != channelType {
			_ = newChannel.Reject(ssh.UnknownChannelType, "Copperline relay accepts only its private relay channel")
			continue
		}
		ch, requests, err := newChannel.Accept()
		if err != nil {
			continue
		}
		go ssh.DiscardRequests(requests)
		peer := &serverPeer{
			server: s, ch: ch, transport: conn, enc: json.NewEncoder(ch), dec: json.NewDecoder(ch),
			sendQ: make(chan frame, 256), closed: make(chan struct{}),
		}
		go peer.writer()
		go peer.serve()
	}
}

func (p *serverPeer) serve() {
	defer p.close()

	var hello frame
	if err := p.dec.Decode(&hello); err != nil || hello.Type != "hello" || hello.Version != protocolVersion {
		_ = p.send(frame{Type: "error", Error: fmt.Sprintf("Copperline relay protocol %d required", protocolVersion)})
		return
	}
	p.notifications = hello.Bool
	if err := p.send(frame{Type: "hello", Version: protocolVersion}); err != nil {
		return
	}

	// Atomically seed this client with retained relay-session history and a
	// state snapshot, then put it on the live broadcast list. onMessage takes
	// the same lock, so no live message can fall into or duplicate this gap.
	p.server.syncMu.Lock()
	history := p.server.replayMessages()
	snapshot := p.server.irc.Snapshot()
	if err := p.send(frame{Type: "history", Messages: history}); err != nil {
		p.server.syncMu.Unlock()
		return
	}
	if err := p.send(frame{Type: "snapshot", Snapshot: &snapshot}); err != nil {
		p.server.syncMu.Unlock()
		return
	}
	if err := p.send(frame{Type: "ready"}); err != nil {
		p.server.syncMu.Unlock()
		return
	}
	p.server.addPeer(p)
	p.server.syncMu.Unlock()

	for {
		var req frame
		if err := p.dec.Decode(&req); err != nil {
			if !errors.Is(err, io.EOF) {
				_ = p.send(frame{Type: "error", Error: err.Error()})
			}
			return
		}
		if req.Type != "request" || req.ID == 0 {
			continue
		}
		go p.handleRequest(req)
	}
}

func (p *serverPeer) handleRequest(req frame) {
	var err error
	var result bool
	m := p.server.irc

	switch req.Action {
	case "ping":
		result = true
	case "connect":
		err = m.ConnectServer(req.Server)
	case "disconnect":
		err = m.DisconnectServer(req.Server, req.Text)
	case "message":
		err = m.SendMessage(req.Server, req.Target, req.Text)
	case "typing":
		result, err = m.SendTyping(req.Server, req.Target, req.Text)
	case "ctcp":
		err = m.SendCTCP(req.Server, req.Target, req.Extra, req.Text)
	case "action":
		err = m.SendAction(req.Server, req.Target, req.Text)
	case "notice":
		err = m.Notice(req.Server, req.Target, req.Text)
	case "join":
		err = m.Join(req.Server, req.Target, req.Extra)
	case "part":
		err = m.Part(req.Server, req.Target, req.Text)
	case "nick":
		err = m.Nick(req.Server, req.Text)
	case "topic":
		err = m.Topic(req.Server, req.Target, req.Text)
	case "whois":
		err = m.Whois(req.Server, req.Target)
	case "raw":
		err = m.Raw(req.Server, req.Text)
	case "history":
		err = m.RequestHistory(req.Server, req.Target, req.Limit)
	case "markread":
		err = m.MarkRead(req.Server, req.Target, req.Text)
	case "dcc_accept":
		err = m.DCCAccept(req.Server, req.Peer, func(line string) {
			_ = p.send(frame{Type: "dcc_chat", Server: req.Server, Peer: req.Peer, Text: line})
		})
	case "dcc_send":
		err = m.DCCSend(req.Server, req.Peer, req.Text)
	default:
		err = fmt.Errorf("unknown relay action %q", req.Action)
	}

	resp := frame{Type: "response", ID: req.ID, Bool: result}
	if err != nil {
		resp.Error = err.Error()
	}
	_ = p.send(resp)
}

func (p *serverPeer) send(f frame) error {
	select {
	case <-p.closed:
		return io.ErrClosedPipe
	default:
	}
	select {
	case <-p.closed:
		return io.ErrClosedPipe
	case p.sendQ <- f:
		return nil
	default:
		// A relay client that cannot consume a bounded queue must not stall IRC
		// processing for every other attached client. Drop the attachment; it can
		// reconnect and receive retained history again.
		p.close()
		return errors.New("relay client is too slow")
	}
}

func (p *serverPeer) writer() {
	for {
		select {
		case <-p.closed:
			return
		case f := <-p.sendQ:
			err := p.writeFrame(f, relayRequestTimeout)
			if err != nil {
				p.close()
				return
			}
		}
	}
}

func (p *serverPeer) writeFrame(f frame, timeout time.Duration) error {
	// A sleeping peer may stop advancing its SSH receive window even before
	// the queue fills. Closing its socket also releases a blocked channel write.
	timer := time.AfterFunc(timeout, p.close)
	defer timer.Stop()
	return p.enc.Encode(f)
}

func (p *serverPeer) close() {
	p.closeOnce.Do(func() {
		close(p.closed)
		p.server.removePeer(p)
		// Channel.Close writes an SSH packet and may block on a sleeping
		// machine. Never do transport cleanup inside broadcast or sync.Once:
		// another caller of close would wait there and stall all IRC clients.
		go func() {
			if p.transport != nil {
				_ = p.transport.Close()
			}
			if p.ch != nil {
				_ = p.ch.Close()
			}
		}()
	})
}

func (s *Server) onMessage(msg model.Message) {
	s.syncMu.Lock()
	msg.RelayID = newMessageID()
	s.state.Add(msg)
	s.historyDirty.Store(true)
	s.deliverMessage(msg, s.irc.CurrentNick(msg.Server))
	s.syncMu.Unlock()

	// Disk logging is not part of the relay delivery critical section. A slow
	// filesystem must not block new SSH attachments or live IRC forwarding.
	_ = s.logger.BeginBuffer(msg.Server, msg.Target)
	_ = s.logger.Write(msg)
}

func (s *Server) onEvent(ev ircclient.Event) {
	// NAMES/WHO/topic synchronization numerics can be enormous on a large
	// channel and contain no presentation information that is not already in
	// the coalesced snapshot stream. Do not turn the old 352/354 flood into an
	// SSH-frame flood. Lifecycle and other meaningful/raw events still cross the
	// relay, so Lua and connection feedback retain useful event visibility.
	if relayHousekeepingNumeric(ev.Command) {
		return
	}
	s.broadcast(frame{Type: "event", Event: &ev})
}

func relayHousekeepingNumeric(command string) bool {
	switch command {
	case "315", "324", "328", "329", "332", "333", "352", "353", "354", "366":
		return true
	default:
		return false
	}
}

func (s *Server) requestSnapshot() {
	select {
	case s.snapshotWake <- struct{}{}:
	default:
	}
}

func (s *Server) snapshotLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.snapshotWake:
			// Collapse IRC bursts (NAMES/WHO in particular) into one state frame.
			timer := time.NewTimer(75 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			for {
				select {
				case <-s.snapshotWake:
				default:
					goto drained
				}
			}
		drained:
			snapshot := s.irc.Snapshot()
			s.broadcast(frame{Type: "snapshot", Snapshot: &snapshot})
		}
	}
}

func (s *Server) replayMessages() []model.Message {
	buffers, _ := s.state.Snapshot()
	var out []model.Message
	for _, b := range buffers {
		for _, msg := range b.Messages {
			msg.Replay = true
			out = append(out, msg)
		}
	}
	return out
}

func (s *Server) addPeer(p *serverPeer) {
	s.peersMu.Lock()
	s.nextPeer++
	p.order = s.nextPeer
	s.peers[p] = struct{}{}
	s.peersMu.Unlock()
}

func (s *Server) removePeer(p *serverPeer) {
	s.peersMu.Lock()
	delete(s.peers, p)
	s.peersMu.Unlock()
}

func (s *Server) closePeers() {
	s.peersMu.Lock()
	peers := make([]*serverPeer, 0, len(s.peers))
	for p := range s.peers {
		peers = append(peers, p)
	}
	s.peersMu.Unlock()
	for _, p := range peers {
		p.close()
	}
}

func (s *Server) broadcast(f frame) {
	s.peersMu.Lock()
	peers := make([]*serverPeer, 0, len(s.peers))
	for p := range s.peers {
		peers = append(peers, p)
	}
	s.peersMu.Unlock()
	for _, p := range peers {
		_ = p.send(f) // send detaches slow peers without waiting for SSH cleanup.
	}
}

package irc

import (
	"copperline/internal/dcc"
	"copperline/internal/model"
)

// Backend is the IRC-facing surface consumed by the TUI. The ordinary Manager
// implements it directly, while the relay client implements the same surface
// over Copperline's SSH relay protocol.
type Backend interface {
	SetMessageSink(func(model.Message))
	SetEventSink(func(Event))
	SetUpdateSink(func())
	Start()
	Stop(reason string)
	ConnectServer(name string) error
	DisconnectServer(name, reason string) error
	SendMessage(server, target, text string) error
	SendTyping(server, target, state string) (bool, error)
	TypingUsers(server, target string) []string
	SendCTCP(server, target, command, text string) error
	SendAction(server, target, text string) error
	Notice(server, target, text string) error
	Join(server, channel, key string) error
	Part(server, channel, reason string) error
	Nick(server, nick string) error
	Topic(server, channel, text string) error
	Whois(server, nick string) error
	Raw(server, line string) error
	RequestHistory(server, target string, limit int) error
	MarkRead(server, target, msgid string) error
	Names(server, channel string) []string
	NickPrefix(server, channel, nick string) string
	ChannelTopic(server, channel string) string
	IsJoined(server, channel string) bool
	ServerNames() []string
	KnownTargets(server string) []string
	CurrentNick(server string) string
	IsConnected(server string) bool
	WantsConnection(server string) bool
	Capabilities(server string) []string
	DCCOffers() []dcc.Offer
	DCCAccept(server, peer string, onChatLine func(string)) error
	DCCSend(server, peer, path string) error
}

// StateSnapshot is the compact state a relay server publishes to attached
// Copperline clients. Messages are streamed separately; this contains only the
// information needed by status bars, topics, nick lists and connection state.
type StateSnapshot struct {
	Servers   []ServerSnapshot `json:"servers"`
	DCCOffers []dcc.Offer      `json:"dcc_offers,omitempty"`
}

type ServerSnapshot struct {
	Name            string           `json:"name"`
	Nick            string           `json:"nick,omitempty"`
	Connected       bool             `json:"connected"`
	WantsConnection bool             `json:"wants_connection"`
	Capabilities    []string         `json:"capabilities,omitempty"`
	Targets         []TargetSnapshot `json:"targets,omitempty"`
}

type TargetSnapshot struct {
	Target      string         `json:"target"`
	Joined      bool           `json:"joined,omitempty"`
	Topic       string         `json:"topic,omitempty"`
	Users       []UserSnapshot `json:"users,omitempty"`
	TypingUsers []string       `json:"typing_users,omitempty"`
}

type UserSnapshot struct {
	Nick   string `json:"nick"`
	Prefix string `json:"prefix,omitempty"`
}

var _ Backend = (*Manager)(nil)

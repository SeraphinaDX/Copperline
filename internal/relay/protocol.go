package relay

import (
	"copperline/internal/irc"
	"copperline/internal/model"
)

const (
	protocolVersion = 2
	channelType     = "copperline-relay@copperline"
)

type frame struct {
	Type    string `json:"type"`
	Version int    `json:"version,omitempty"`
	ID      uint64 `json:"id,omitempty"`
	Action  string `json:"action,omitempty"`
	Error   string `json:"error,omitempty"`
	Bool    bool   `json:"bool,omitempty"`

	Server string `json:"server,omitempty"`
	Target string `json:"target,omitempty"`
	Text   string `json:"text,omitempty"`
	Extra  string `json:"extra,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Peer   string `json:"peer,omitempty"`

	Message  *model.Message     `json:"message,omitempty"`
	Messages []model.Message    `json:"messages,omitempty"`
	Event    *irc.Event         `json:"event,omitempty"`
	Snapshot *irc.StateSnapshot `json:"snapshot,omitempty"`
}

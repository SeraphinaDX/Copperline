package irc

import (
	"slices"
	"strings"

	"copperline/internal/model"
	"github.com/lrstanley/girc"
)

type channelJoin struct {
	channel string
	key     string
}

// Only confirmed self-JOINs extend the reconnect list. An open buffer, another
// user's JOIN, or a rejected JOIN request is not evidence of our membership.
// Configured autojoins seed the list once, when the Session is created.
func (s *Session) confirmChannelJoin(channels string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, channel := range strings.Split(channels, ",") {
		if !model.IsChannel(channel) {
			continue
		}
		id := girc.ToRFC1459(channel)
		if s.pendingParts[id] {
			// A late JOIN echo must not undo a leave already requested locally.
			continue
		}
		key, requested := s.pendingJoinKeys[id]
		delete(s.pendingJoinKeys, id)
		index := slices.IndexFunc(s.rejoinChannels, func(j channelJoin) bool {
			return girc.ToRFC1459(j.channel) == id
		})
		if index < 0 {
			s.rejoinChannels = append(s.rejoinChannels, channelJoin{channel, key})
		} else if requested {
			s.rejoinChannels[index].key = key
		}
	}
}

func (s *Session) rememberJoinRequest(channels, keys string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pendingJoinKeys == nil {
		s.pendingJoinKeys = make(map[string]string)
	}
	keyList := strings.Split(keys, ",")
	for i, channel := range strings.Split(channels, ",") {
		if !model.IsChannel(channel) {
			continue
		}
		id := girc.ToRFC1459(channel)
		key := ""
		if i < len(keyList) {
			key = keyList[i]
		}
		s.pendingJoinKeys[id] = key
		delete(s.pendingParts, id)
	}
}

func (s *Session) forgetChannels(channels string, awaitingPart bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, channel := range strings.Split(channels, ",") {
		id := girc.ToRFC1459(channel)
		s.rejoinChannels = slices.DeleteFunc(s.rejoinChannels, func(j channelJoin) bool {
			return girc.ToRFC1459(j.channel) == id
		})
		delete(s.pendingJoinKeys, id)
		if awaitingPart {
			if s.pendingParts == nil {
				s.pendingParts = make(map[string]bool)
			}
			s.pendingParts[id] = true
		} else {
			delete(s.pendingParts, id)
		}
	}
}

func (s *Session) clearPendingJoins() {
	s.mu.Lock()
	defer s.mu.Unlock()
	clear(s.pendingJoinKeys)
	clear(s.pendingParts)
}

func (s *Session) channelsToRejoin() []channelJoin {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]channelJoin(nil), s.rejoinChannels...)
}

func (s *Session) rejoin(c *girc.Client) {
	var unkeyed []string
	for _, j := range s.channelsToRejoin() {
		if j.key != "" {
			c.Cmd.JoinKey(j.channel, j.key)
		} else {
			unkeyed = append(unkeyed, j.channel)
		}
	}
	c.Cmd.Join(unkeyed...)
}

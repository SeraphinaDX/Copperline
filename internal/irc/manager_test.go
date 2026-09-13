package irc

import (
	"testing"

	"copperline/internal/config"
	"copperline/internal/model"

	"github.com/lrstanley/girc"
)

func TestChannelHousekeepingNumericsAreHidden(t *testing.T) {
	hidden := []string{"315", "324", "328", "329", "332", "333", "352", "354", "353", "366"}
	for _, command := range hidden {
		if !isChannelHousekeepingNumeric(command) {
			t.Errorf("numeric %s should be treated as channel housekeeping", command)
		}
	}

	visible := []string{"001", "301", "311", "367", "368", "473", "475", "477"}
	for _, command := range visible {
		if isChannelHousekeepingNumeric(command) {
			t.Errorf("numeric %s should remain visible", command)
		}
	}
}

func TestOnlyJoinPartAndQuitSuppressUnreadActivity(t *testing.T) {
	var messages []model.Message
	m := &Manager{
		cfg:          &config.Config{},
		sessions:     make(map[string]*Session),
		typing:       make(map[string]typingEntry),
		knownTargets: make(map[string]map[string]struct{}),
		emit:         func(msg model.Message) { messages = append(messages, msg) },
	}
	c := girc.New(girc.Config{Server: "irc.test", Nick: "tester", User: "tester"})
	source := &girc.Source{Name: "alice"}

	cases := []struct {
		command  string
		params   []string
		suppress bool
	}{
		{girc.JOIN, []string{"#chan"}, true},
		{girc.PART, []string{"#chan", "bye"}, true},
		{girc.QUIT, []string{"gone"}, true},
		{girc.KICK, []string{"#chan", "bob", "reason"}, false},
		{girc.TOPIC, []string{"#chan", "new topic"}, false},
	}
	for _, tc := range cases {
		messages = nil
		m.handleEvent("test", c, girc.Event{Command: tc.command, Params: tc.params, Source: source})
		if len(messages) != 1 {
			t.Fatalf("%s emitted %d messages, want 1: %#v", tc.command, len(messages), messages)
		}
		if messages[0].SuppressUnread != tc.suppress {
			t.Fatalf("%s SuppressUnread = %t, want %t", tc.command, messages[0].SuppressUnread, tc.suppress)
		}
	}
}

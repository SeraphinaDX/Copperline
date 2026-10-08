package irc

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"copperline/internal/config"
	"github.com/lrstanley/girc"
)

func rejoinTestManager(t *testing.T) *Manager {
	t.Helper()
	no := false
	m := New(&config.Config{
		General: config.GeneralConfig{ShowJoinMessages: &no, ShowPartMessages: &no, IgnoreFile: t.TempDir() + "/ignore.toml"},
		Servers: []config.ServerConfig{
			{Name: "test", Host: "irc.test", Nick: "tester", User: "tester", Channels: []string{"#auto", "#AUTO"}},
			{Name: "other", Host: "irc.other.test", Nick: "tester", User: "tester"},
		},
	}, nil)
	t.Cleanup(m.urls.Close)
	return m
}

func membershipEvent(t *testing.T, m *Manager, server, line string) {
	t.Helper()
	e := girc.ParseEvent(line)
	if e == nil {
		t.Fatalf("invalid test event: %q", line)
	}
	m.handleEvent(server, m.sessions[server].client, *e)
}

func TestRejoinTracksOurMembershipIndependentlyOfBuffers(t *testing.T) {
	m := rejoinTestManager(t)
	s := m.sessions["test"]
	m.rememberTarget("test", "#open-but-not-joined")
	m.rememberTarget("test", "alice")
	membershipEvent(t, m, "test", ":alice!user@host JOIN #someone-elses-channel")
	membershipEvent(t, m, "test", ":TeStEr!user@host JOIN #Manual")
	membershipEvent(t, m, "test", ":tester!user@host JOIN #MANUAL")
	membershipEvent(t, m, "test", ":alice!user@host PART #Manual :netsplit")
	membershipEvent(t, m, "test", ":alice!user@host QUIT :a.example b.example")
	membershipEvent(t, m, "test", ":op!user@host KICK #Manual alice :bye")
	membershipEvent(t, m, "other", ":tester!user@host JOIN #OtherNetwork")
	membershipEvent(t, m, "test", ":tester!user@host JOIN #departed")
	membershipEvent(t, m, "test", ":tester!user@host PART #departed :bye")
	membershipEvent(t, m, "test", ":tester!user@host JOIN #kicked")
	membershipEvent(t, m, "test", ":op!user@host KICK #kicked TeStEr :bye")
	m.handleEvent("test", s.client, girc.Event{Command: girc.DISCONNECTED})
	want := []channelJoin{{"#auto", ""}, {"#Manual", ""}}
	if got := s.channelsToRejoin(); !reflect.DeepEqual(got, want) {
		t.Fatalf("rejoin channels = %#v, want %#v", got, want)
	}
	if got := m.sessions["other"].channelsToRejoin(); !reflect.DeepEqual(got, []channelJoin{{"#OtherNetwork", ""}}) {
		t.Fatalf("other network = %#v", got)
	}
}

func wireJoins(t *testing.T, c *girc.Client, lines <-chan string) map[string]string {
	t.Helper()
	// A round trip fences JOIN commands, so inspecting the wire never relies
	// on sleeps or on whether the send goroutine has consumed its queue yet.
	if err := confirmSend(c, func() {}, time.Second); err != nil {
		t.Fatal(err)
	}
	joins := map[string]string{}
	for len(lines) > 0 {
		e := girc.ParseEvent(<-lines)
		if e == nil || e.Command != girc.JOIN {
			continue
		}
		keys := []string{}
		if len(e.Params) > 1 {
			keys = strings.Split(e.Params[1], ",")
		}
		for i, channel := range strings.Split(e.Params[0], ",") {
			id := girc.ToRFC1459(channel)
			if _, exists := joins[id]; exists {
				t.Fatalf("duplicate JOIN for %q", channel)
			}
			key := ""
			if i < len(keys) {
				key = keys[i]
			}
			joins[id] = key
		}
	}
	return joins
}

func TestReconnectRejoinsManualChannelsAndKeysOnTheWire(t *testing.T) {
	m := rejoinTestManager(t)
	c := m.sessions["test"].client
	c.Config.AllowFlood = true
	first, disconnect := mockIRCWithDisconnect(t, c, "pong")
	c.RunHandlers(&girc.Event{Command: girc.CONNECTED})
	if got := wireJoins(t, c, first); !reflect.DeepEqual(got, map[string]string{"#auto": ""}) {
		t.Fatalf("initial autojoin = %#v", got)
	}
	c.RunHandlers(girc.ParseEvent(":tester!user@host JOIN #auto"))
	if err := m.Join("test", "#Manual", ""); err != nil {
		t.Fatal(err)
	}
	c.RunHandlers(girc.ParseEvent(":tester!user@host JOIN #Manual"))
	if err := m.Join("test", "#Locked", "secret-key"); err != nil {
		t.Fatal(err)
	}
	c.RunHandlers(girc.ParseEvent(":tester!user@host JOIN #Locked"))
	// A failed join leaves an open buffer but must not become membership.
	if err := m.Join("test", "#rejected", "wrong-key"); err != nil {
		t.Fatal(err)
	}
	c.RunHandlers(girc.ParseEvent(":irc.test 475 tester #rejected :Bad channel key"))
	wireJoins(t, c, first)
	if c.LookupChannel("#Manual") == nil {
		t.Fatal("manual channel was never joined")
	}
	disconnect()

	for attempt := 0; attempt < 2; attempt++ {
		lines, closeConnection := mockIRCWithDisconnect(t, c, "pong")
		if c.LookupChannel("#Manual") != nil {
			t.Fatal("test did not reset girc's connection membership")
		}
		c.RunHandlers(&girc.Event{Command: girc.CONNECTED})
		want := map[string]string{"#auto": "", "#manual": "", "#locked": "secret-key"}
		if got := wireJoins(t, c, lines); !reflect.DeepEqual(got, want) {
			t.Fatalf("reconnect %d JOINs = %#v, want %#v", attempt, got, want)
		}
		for _, channel := range []string{"#auto", "#Manual", "#Locked"} {
			c.RunHandlers(girc.ParseEvent(":tester!user@host JOIN " + channel))
		}
		closeConnection()
	}
}

func TestPartRequestSurvivesLostEchoAndDoesNotUndoLeave(t *testing.T) {
	m := rejoinTestManager(t)
	c := m.sessions["test"].client
	c.Config.AllowFlood = true
	lines := mockIRC(t, c, "pong")
	c.RunHandlers(&girc.Event{Command: girc.CONNECTED})
	if err := m.Join("test", "#manual", ""); err != nil {
		t.Fatal(err)
	}
	for _, channel := range []string{"#auto", "#manual"} {
		c.RunHandlers(girc.ParseEvent(":tester!user@host JOIN " + channel))
		if err := m.Part("test", channel, "closing"); err != nil {
			t.Fatal(err)
		}
		// A JOIN arriving while PART is queued must not resurrect the channel.
		c.RunHandlers(girc.ParseEvent(":tester!user@host JOIN " + channel))
	}
	wireJoins(t, c, lines)
	m.handleEvent("test", c, girc.Event{Command: girc.DISCONNECTED})
	if got := m.sessions["test"].channelsToRejoin(); len(got) != 0 {
		t.Fatalf("parted channels reappeared after losing PART echo: %#v", got)
	}
	if err := m.Join("test", "#manual", "new-key"); err == nil {
		t.Fatal("join while IRC disconnected unexpectedly succeeded")
	}
	c.RunHandlers(&girc.Event{Command: girc.CONNECTED})
	if err := m.Join("test", "#manual", "new-key"); err != nil {
		t.Fatal(err)
	}
	c.RunHandlers(girc.ParseEvent(":tester!user@host JOIN #manual"))
	if got := m.sessions["test"].channelsToRejoin(); !reflect.DeepEqual(got, []channelJoin{{"#manual", "new-key"}}) {
		t.Fatalf("explicit rejoin after PART = %#v", got)
	}
}

func TestRejoinConcurrentMembershipAndSnapshots(t *testing.T) {
	s := &Session{}
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		for range 100 {
			s.rememberJoinRequest("#chat", "key")
			s.confirmChannelJoin("#chat")
			s.forgetChannels("#chat", false)
		}
	}()
	for range 100 {
		s.channelsToRejoin()
		s.clearPendingJoins()
	}
	<-finished
}

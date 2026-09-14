package irc

import (
	"strings"
	"testing"
)

func TestParseWhoisReply(t *testing.T) {
	tests := []struct {
		command string
		params  []string
		want    string
		end     bool
	}{
		{"311", []string{"me", "Alice", "user", "host.example", "*", "Alice Example"}, "Alice is user@host.example", false},
		{"312", []string{"me", "Alice", "irc.example", "Example server"}, "connected to irc.example", false},
		{"317", []string{"me", "Alice", "90", "1700000000", "seconds idle"}, "idle: 1m30s", false},
		{"319", []string{"me", "Alice", "@#one +#two"}, "channels: @#one +#two", false},
		{"330", []string{"me", "Alice", "alice-account", "is logged in as"}, "logged in as alice-account", false},
		{"671", []string{"me", "Alice", "is using a secure connection"}, "secure connection", false},
		{"276", []string{"me", "Alice", "has client certificate fingerprint abc123"}, "certificate:", false},
		{"318", []string{"me", "Alice", "End of /WHOIS list."}, "End of WHOIS", true},
		{"401", []string{"me", "Missing", "No such nick"}, "No such nickname", true},
	}
	for _, tc := range tests {
		t.Run(tc.command, func(t *testing.T) {
			reply, ok := ParseWhoisReply(Event{Command: tc.command, Params: tc.params})
			if !ok {
				t.Fatal("reply was not recognized")
			}
			if !strings.Contains(reply.Text, tc.want) || reply.End != tc.end {
				t.Fatalf("reply = %#v, want text containing %q and end=%t", reply, tc.want, tc.end)
			}
		})
	}
	if _, ok := ParseWhoisReply(Event{Command: "332", Params: []string{"me", "#channel", "topic"}}); ok {
		t.Fatal("channel numeric was recognized as WHOIS")
	}
}

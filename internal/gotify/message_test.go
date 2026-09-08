package gotify

import (
	"copperline/internal/config"
	"copperline/internal/model"
	"testing"
)

func TestNotificationEligibility(t *testing.T) {
	no := false
	for _, tc := range []struct {
		name string
		msg  model.Message
		cfg  config.GotifyConfig
		want bool
	}{
		{"mention", model.Message{Nick: "alice", Target: "#go", Text: "Hi me!", Kind: model.KindMessage}, config.GotifyConfig{}, true},
		{"substring", model.Message{Nick: "alice", Target: "#go", Text: "theme", Kind: model.KindMessage}, config.GotifyConfig{}, false},
		{"pm", model.Message{Nick: "alice", Target: "alice", Text: "hello", Kind: model.KindMessage}, config.GotifyConfig{}, true},
		{"action", model.Message{Nick: "alice", Target: "#go", Text: "waves at me", Kind: model.KindAction}, config.GotifyConfig{}, true},
		{"self", model.Message{Nick: "ME", Target: "alice", Text: "me", Kind: model.KindMessage}, config.GotifyConfig{}, false},
		{"replay", model.Message{Nick: "alice", Target: "alice", Text: "hello", Kind: model.KindMessage, Replay: true}, config.GotifyConfig{}, false},
		{"non-owner", model.Message{Nick: "alice", Target: "alice", Text: "hello", Kind: model.KindMessage, SuppressNotify: true}, config.GotifyConfig{}, false},
		{"disabled-pm", model.Message{Nick: "alice", Target: "alice", Text: "hello", Kind: model.KindMessage}, config.GotifyConfig{PrivateMessages: &no}, false},
		{"system", model.Message{Nick: "alice", Target: "alice", Text: "hello", Kind: model.KindSystem}, config.GotifyConfig{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			title, body := Notification(tc.cfg, tc.msg, "me")
			if (title != "" && body != "") != tc.want {
				t.Fatalf("notification = %q %q", title, body)
			}
		})
	}
}

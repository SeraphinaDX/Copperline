package gotify

import (
	"fmt"
	"strings"

	"copperline/internal/config"
	"copperline/internal/model"
)

// Notification applies the same eligibility rules in direct and relay modes.
func Notification(cfg config.GotifyConfig, msg model.Message, self string) (title, body string) {
	if msg.Replay || msg.SuppressNotify || self == "" || msg.Nick == "" || strings.EqualFold(msg.Nick, self) {
		return "", ""
	}
	if msg.Kind != model.KindMessage && msg.Kind != model.KindAction {
		return "", ""
	}
	body = fmt.Sprintf("<%s> %s", msg.Nick, msg.Text)
	if msg.Kind == model.KindAction {
		body = fmt.Sprintf("* %s %s", msg.Nick, msg.Text)
	}
	if (msg.Mention || model.ContainsNickMention(msg.Text, self)) && cfg.MentionsEnabled() {
		return fmt.Sprintf("Copperline mention: %s / %s", msg.Server, msg.Target), body
	}
	if msg.Target != "" && msg.Target != "*server*" && !model.IsChannel(msg.Target) && cfg.PrivateMessagesEnabled() {
		return fmt.Sprintf("Copperline PM: %s on %s", msg.Nick, msg.Server), body
	}
	return "", ""
}

func (n *Notifier) NotifyMessage(cfg config.GotifyConfig, msg model.Message, self string) {
	if !n.Enabled() {
		return
	}
	title, body := Notification(cfg, msg, self)
	if title != "" {
		n.Send(title, body)
	}
}

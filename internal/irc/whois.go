package irc

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// WhoisReply is presentation-ready WHOIS information decoded from an IRC
// numeric. End is set for replies which finish the pending request.
type WhoisReply struct {
	Nick string
	Text string
	End  bool
}

// ParseWhoisReply recognizes standard WHOIS numerics plus common extensions.
func ParseWhoisReply(ev Event) (WhoisReply, bool) {
	if len(ev.Params) < 2 {
		return WhoisReply{}, false
	}
	nick := ev.Params[1]
	last := ev.Params[len(ev.Params)-1]
	reply := WhoisReply{Nick: nick}
	switch ev.Command {
	case "301":
		reply.Text = fmt.Sprintf("%s is away: %s", nick, last)
	case "307":
		reply.Text = fmt.Sprintf("%s is a registered nickname", nick)
	case "310":
		reply.Text = fmt.Sprintf("%s is available for help", nick)
	case "311":
		if len(ev.Params) < 6 {
			return WhoisReply{}, false
		}
		reply.Text = fmt.Sprintf("%s is %s@%s — %s", nick, ev.Params[2], ev.Params[3], last)
	case "312":
		if len(ev.Params) < 4 {
			return WhoisReply{}, false
		}
		reply.Text = fmt.Sprintf("%s is connected to %s — %s", nick, ev.Params[2], last)
	case "313":
		reply.Text = fmt.Sprintf("%s is an IRC operator", nick)
	case "317":
		if len(ev.Params) < 4 {
			return WhoisReply{}, false
		}
		idle, err := strconv.ParseInt(ev.Params[2], 10, 64)
		if err != nil {
			reply.Text = fmt.Sprintf("%s idle: %s seconds", nick, ev.Params[2])
		} else {
			reply.Text = fmt.Sprintf("%s idle: %s", nick, (time.Duration(idle) * time.Second).String())
		}
		if signedOn, err := strconv.ParseInt(ev.Params[3], 10, 64); err == nil && signedOn > 0 {
			reply.Text += "; signed on " + time.Unix(signedOn, 0).Local().Format("2006-01-02 15:04:05")
		}
	case "318":
		reply.Text = "End of WHOIS for " + nick
		reply.End = true
	case "319":
		reply.Text = fmt.Sprintf("%s channels: %s", nick, last)
	case "320", "335", "338", "378", "379":
		reply.Text = fmt.Sprintf("%s: %s", nick, last)
	case "330":
		if len(ev.Params) < 4 {
			return WhoisReply{}, false
		}
		reply.Text = fmt.Sprintf("%s is logged in as %s", nick, ev.Params[2])
	case "671":
		reply.Text = fmt.Sprintf("%s is using a secure connection", nick)
	case "276":
		reply.Text = fmt.Sprintf("%s certificate: %s", nick, last)
	case "401":
		reply.Text = fmt.Sprintf("No such nickname: %s", nick)
		reply.End = true
	default:
		return WhoisReply{}, false
	}
	reply.Text = strings.TrimSpace(reply.Text)
	return reply, reply.Text != ""
}

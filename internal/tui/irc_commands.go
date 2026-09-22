package tui

import (
	"fmt"
	"strconv"
	"strings"

	"copperline/internal/model"
	"github.com/lrstanley/girc"
)

func (a *App) executeIRCCommand(b *model.Buffer, cmd, rest string) {
	event, err := ircCommand(cmd, rest, b.Target, a.irc.CurrentNick(b.Server))
	if err != nil {
		a.local(b.Server, b.Target, model.KindError, err.Error())
		return
	}
	// Register before sending: a direct or relay backend can deliver the reply
	// before Raw returns.
	pending := a.trackIRCQuery(b.Server, b.Target, event)
	if err := a.irc.Raw(b.Server, event.String()); err != nil {
		a.forgetIRCQuery(pending)
		text := err.Error()
		if cmd == "oper" {
			// A raw-send error can include the entire protocol line.
			text = "could not send OPER; check the server connection"
		}
		a.local(b.Server, b.Target, model.KindError, text)
	}
}

// ircCommand builds parameters separately so a leading colon in a password or
// free-form text remains literal, and tabs between arguments never reach IRC.
func ircCommand(cmd, rest, target, nick string) (*girc.Event, error) {
	usage := map[string]string{
		"oper": "name password", "away": "[message]", "back": "",
		"mode": "[target] [modes [arguments]]", "op": "[#channel] nick",
		"deop": "[#channel] nick", "voice": "[#channel] nick", "devoice": "[#channel] nick",
		"kick": "[#channel] nick [reason]", "ban": "[#channel] [mask]", "unban": "[#channel] mask",
		"invite": "nick [#channel]", "list": "[channels [server]]", "names": "[#channel]",
		"who": "[mask [flags]]", "whowas": "nick [count [server]]",
		"motd": "[server]", "time": "[server]", "stats": "[query [server]]", "links": "[mask | server mask]",
	}
	syntax, ok := usage[cmd]
	if !ok {
		return nil, fmt.Errorf("unknown command /%s", cmd)
	}
	badUsage := fmt.Errorf("usage: %s", strings.TrimSpace("/"+cmd+" "+syntax))
	if strings.ContainsAny(rest, "\r\n\x00") {
		return nil, fmt.Errorf("/%s arguments must not contain CR, LF, or NUL", cmd)
	}
	args := strings.Fields(rest)
	event := &girc.Event{Command: strings.ToUpper(cmd)}
	switch cmd {
	case "oper":
		name, password := cutWord(rest)
		if name == "" || password == "" {
			return nil, badUsage
		}
		event.Params = []string{name, password}
	case "away", "back":
		event.Command = "AWAY"
		if cmd == "back" && len(args) != 0 {
			return nil, badUsage
		}
		if rest != "" {
			event.Params = []string{rest}
		}
	case "mode":
		// Test mode signs before IsChannel: '+' is also a channel prefix.
		if len(args) == 0 || strings.HasPrefix(args[0], "+") || strings.HasPrefix(args[0], "-") {
			if !model.IsChannel(target) {
				target = nick
			}
			if target == "" {
				return nil, badUsage
			}
			args = append([]string{target}, args...)
		}
		event.Params = args
	case "op", "deop", "voice", "devoice", "kick", "ban", "unban", "names":
		arg, tail := cutWord(rest)
		if model.IsChannel(arg) {
			target, rest = arg, tail
			arg, tail = cutWord(rest)
		}
		if !model.IsChannel(target) || strings.Contains(target, ",") {
			return nil, badUsage
		}
		switch cmd {
		case "names":
			if arg != "" {
				return nil, badUsage
			}
			event.Params = []string{target}
		case "kick":
			if arg == "" {
				return nil, badUsage
			}
			event.Params = []string{target, arg}
			if tail != "" {
				event.Params = append(event.Params, tail)
			}
		default:
			if tail != "" || (arg == "" && cmd != "ban") {
				return nil, badUsage
			}
			mode := map[string]string{"op": "+o", "deop": "-o", "voice": "+v", "devoice": "-v", "ban": "+b", "unban": "-b"}[cmd]
			event.Command, event.Params = "MODE", []string{target, mode}
			if arg != "" {
				event.Params = append(event.Params, arg)
			}
		}
	case "invite":
		if len(args) == 2 {
			if model.IsChannel(args[0]) { // Also accept the original channel-first spelling.
				args[0], args[1] = args[1], args[0]
			}
			target = args[1]
		} else if len(args) != 1 {
			return nil, badUsage
		}
		if !model.IsChannel(target) || model.IsChannel(args[0]) || strings.Contains(target, ",") {
			return nil, badUsage
		}
		event.Params = []string{args[0], target}
	case "who":
		if len(args) > 2 {
			return nil, badUsage
		}
		if len(args) == 0 {
			if !model.IsChannel(target) {
				target = "*"
			}
			args = []string{target}
		}
		event.Params = args
	case "whowas":
		if len(args) < 1 || len(args) > 3 {
			return nil, badUsage
		}
		if len(args) > 1 {
			count, err := strconv.Atoi(args[1])
			if err != nil || count < 0 {
				return nil, badUsage
			}
		}
		event.Params = args
	default:
		maxArgs := 2
		if cmd == "motd" || cmd == "time" {
			maxArgs = 1
		}
		if len(args) > maxArgs {
			return nil, badUsage
		}
		event.Params = args
	}
	// Only these commands have a free-form final parameter. In particular, a
	// stray ':' in a mode argument must not absorb subsequent arguments.
	for i, param := range event.Params {
		freeText := i == len(event.Params)-1 && (cmd == "oper" || cmd == "away" || (cmd == "kick" && len(event.Params) == 3))
		if param == "" || strings.ContainsAny(param, "\r\n\x00") || (!freeText && (strings.HasPrefix(param, ":") || strings.ContainsAny(param, " \t"))) {
			return nil, badUsage
		}
	}
	if len(event.Params) > 15 || event.Len() > 510 {
		return nil, fmt.Errorf("/%s exceeds the IRC message limit", cmd)
	}
	return event, nil
}

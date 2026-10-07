// Package clientcmd contains frontend-independent relay/IRC commands.
package clientcmd

import (
	"fmt"
	"strings"

	"copperline/internal/irc"
	"copperline/internal/model"
	"copperline/internal/sysinfo"
)

type Context struct {
	Backend        irc.Backend
	Server, Target string
	Open           func(server, target string)
	Clear          func()
}

func CutWord(line string) (string, string) {
	line = strings.TrimSpace(line)
	if i := strings.IndexAny(line, " \t"); i >= 0 {
		return line[:i], strings.TrimSpace(line[i:])
	}
	return line, ""
}

// SendText handles the text commands shared with the TUI. Failed sends are
// returned to the frontend, which must retain the draft and never auto-retry it.
func SendText(c Context, line string) (bool, error) {
	if !strings.HasPrefix(line, "/") {
		return false, nil
	}
	cmd, rest := CutWord(strings.TrimPrefix(line, "/"))
	cmd = strings.ToLower(cmd)
	if cmd != "msg" && cmd != "me" && cmd != "notice" && cmd != "flex" {
		return false, nil
	}
	if c.Server == "" {
		return true, fmt.Errorf("select a channel or query first")
	}
	arg, tail := CutWord(rest)
	switch cmd {
	case "flex":
		if rest != "" || strings.ContainsAny(line, "\r\n\x00") {
			return true, fmt.Errorf("usage: /flex")
		}
		if c.Target == "" || c.Target == "*server*" {
			return true, fmt.Errorf("select a channel or query first")
		}
		return true, c.Backend.SendMessage(c.Server, c.Target, sysinfo.Summary())
	case "msg":
		if arg == "" || tail == "" {
			return true, fmt.Errorf("usage: /msg nick message")
		}
		if c.Open != nil {
			c.Open(c.Server, arg)
		}
		return true, c.Backend.SendMessage(c.Server, arg, tail)
	case "me":
		return true, c.Backend.SendAction(c.Server, c.Target, rest)
	default:
		if arg == "" || tail == "" {
			return true, fmt.Errorf("usage: /notice target message")
		}
		return true, c.Backend.Notice(c.Server, arg, tail)
	}
}

func Execute(c Context, line string) error {
	if strings.ContainsAny(line, "\r\n\x00") {
		return fmt.Errorf("send one line at a time")
	}
	if strings.TrimSpace(line) == "" {
		return nil
	}
	if !strings.HasPrefix(line, "/") {
		if c.Target == "*server*" || c.Target == "" {
			return fmt.Errorf("select a channel or query first")
		}
		return c.Backend.SendMessage(c.Server, c.Target, line)
	}
	if handled, err := SendText(c, line); handled {
		return err
	}
	cmd, rest := CutWord(strings.TrimPrefix(line, "/"))
	arg, tail := CutWord(rest)
	switch strings.ToLower(cmd) {
	case "join":
		if !model.IsChannel(arg) {
			return fmt.Errorf("usage: /join #channel [key]")
		}
		err := c.Backend.Join(c.Server, arg, tail)
		if err == nil && c.Open != nil {
			c.Open(c.Server, arg)
		}
		return err
	case "part":
		target, reason := c.Target, rest
		if model.IsChannel(arg) {
			target, reason = arg, tail
		}
		if !model.IsChannel(target) {
			return fmt.Errorf("usage: /part [#channel] [reason]")
		}
		return c.Backend.Part(c.Server, target, reason)
	case "nick":
		if arg == "" || tail != "" {
			return fmt.Errorf("usage: /nick nickname")
		}
		return c.Backend.Nick(c.Server, arg)
	case "whois":
		if arg == "" || tail != "" {
			return fmt.Errorf("usage: /whois nickname")
		}
		return c.Backend.Whois(c.Server, arg)
	case "topic":
		if !model.IsChannel(c.Target) {
			return fmt.Errorf("select a channel first")
		}
		return c.Backend.Topic(c.Server, c.Target, rest)
	case "map":
		if rest != "" {
			return fmt.Errorf("usage: /map")
		}
		if c.Server == "" {
			return fmt.Errorf("select a server first")
		}
		return c.Backend.Raw(c.Server, "MAP")
	case "raw":
		if rest == "" {
			return fmt.Errorf("usage: /raw IRC command")
		}
		return c.Backend.Raw(c.Server, rest)
	case "clear":
		if c.Clear != nil {
			c.Clear()
		}
		return nil
	default:
		return fmt.Errorf("unknown GUI command /%s; use /join, /part, /msg, /me, /notice, /flex, /nick, /topic, /whois, /map, /raw, /clear", cmd)
	}
}

package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"copperline/internal/config"
	"copperline/internal/model"
	"copperline/internal/scripting"
)

func (a *App) execute(line string) {
	b := a.state.Current()
	if b == nil {
		return
	}
	if !strings.HasPrefix(line, "/") {
		// Sending a message always resumes live-follow. This guarantees the
		// server's echo of our message is brought into view even if manual
		// scrolling previously left the transcript's follow flag stale.
		a.follow = true
		a.scrollTranscriptBottom()
		if err := a.irc.SendMessage(b.Server, b.Target, line); err != nil {
			a.local(b.Server, b.Target, model.KindError, err.Error())
		}
		return
	}

	if handled, err := a.sendTextCommand(line); handled {
		if err != nil {
			a.local(b.Server, b.Target, model.KindError, err.Error())
		}
		return
	}

	cmdline := strings.TrimSpace(strings.TrimPrefix(line, "/"))
	cmd, rest := cutWord(cmdline)
	cmd = strings.ToLower(cmd)
	arg1, tail := cutWord(rest)

	switch cmd {
	case "search":
		a.startSearch(rest)
	case "searchnext":
		a.moveSearch(1)
	case "searchprev":
		a.moveSearch(-1)
	case "unread":
		a.selectNextUnread()
	case "help":
		a.local(b.Server, b.Target, model.KindSystem, "commands: /server /buffer /connect /disconnect /join /part /query /msg /me /notice /ctcp /nick /topic /whois /raw /history /search /searchnext /searchprev /unread /markread /caps /dcc /gotify /lua /close /quit")
	case "server":
		if arg1 == "" {
			a.local(b.Server, b.Target, model.KindSystem, "servers: "+strings.Join(a.irc.ServerNames(), ", "))
			return
		}
		a.state.Select(arg1, "*server*")
		a.follow = true
	case "buffer", "buf":
		if arg1 == "" {
			a.local(b.Server, b.Target, model.KindError, "usage: /buffer number")
			return
		}
		n, err := strconv.Atoi(arg1)
		if err != nil || !a.selectBufferNumber(n) {
			a.local(b.Server, b.Target, model.KindError, "no buffer numbered "+arg1)
		}
	case "connect":
		name := arg1
		if name == "" {
			name = b.Server
		}
		if err := a.irc.ConnectServer(name); err != nil {
			a.local(b.Server, b.Target, model.KindError, err.Error())
		}
	case "disconnect":
		name := arg1
		if name == "" {
			name = b.Server
		}
		if err := a.irc.DisconnectServer(name, strings.TrimSpace(tail)); err != nil {
			a.local(b.Server, b.Target, model.KindError, err.Error())
		}
	case "join", "j":
		if arg1 == "" {
			a.local(b.Server, b.Target, model.KindError, "usage: /join #channel [key]")
			return
		}
		key, _ := cutWord(tail)
		a.reopenBuffer(b.Server, arg1)
		a.state.Ensure(b.Server, arg1)
		a.state.Select(b.Server, arg1)
		a.local(b.Server, arg1, model.KindSystem, "joining "+arg1+"...")
		if err := a.irc.Join(b.Server, arg1, key); err != nil {
			a.local(b.Server, arg1, model.KindError, err.Error())
		}
	case "part", "leave":
		channel := arg1
		reason := tail
		if channel == "" {
			channel = b.Target
			reason = rest
		}
		if !model.IsChannel(channel) {
			a.local(b.Server, b.Target, model.KindError, "not a channel")
			return
		}
		if err := a.irc.Part(b.Server, channel, strings.TrimSpace(reason)); err != nil {
			a.local(b.Server, b.Target, model.KindError, err.Error())
		}
	case "query", "q":
		if arg1 != "" {
			a.reopenBuffer(b.Server, arg1)
			a.state.Select(b.Server, arg1)
			a.follow = true
		}
	case "ctcp":
		ctcpCommand, ctcpText := cutWord(tail)
		if arg1 == "" || ctcpCommand == "" {
			a.local(b.Server, b.Target, model.KindError, "usage: /ctcp nick command [text]")
			return
		}
		a.state.Ensure(b.Server, arg1)
		if err := a.irc.SendCTCP(b.Server, arg1, ctcpCommand, strings.TrimSpace(ctcpText)); err != nil {
			a.local(b.Server, arg1, model.KindError, err.Error())
		}
	case "nick":
		if arg1 != "" {
			if err := a.irc.Nick(b.Server, arg1); err != nil {
				a.local(b.Server, b.Target, model.KindError, err.Error())
			}
		}
	case "topic":
		if !model.IsChannel(b.Target) {
			a.local(b.Server, b.Target, model.KindError, "select a channel first")
			return
		}
		if err := a.irc.Topic(b.Server, b.Target, rest); err != nil {
			a.local(b.Server, b.Target, model.KindError, err.Error())
		}
	case "whois":
		if arg1 != "" {
			if err := a.irc.Whois(b.Server, arg1); err != nil {
				a.local(b.Server, b.Target, model.KindError, err.Error())
			}
		}
	case "raw", "quote":
		if err := a.irc.Raw(b.Server, rest); err != nil {
			a.local(b.Server, b.Target, model.KindError, err.Error())
		}
	case "history":
		limit := 50
		if arg1 != "" {
			if n, err := strconv.Atoi(arg1); err == nil && n > 0 {
				limit = n
			}
		}
		if err := a.irc.RequestHistory(b.Server, b.Target, limit); err != nil {
			a.local(b.Server, b.Target, model.KindError, err.Error())
		}
	case "markread":
		a.state.MarkReadThrough(b.Server, b.Target, b.Total)
		msgid := ""
		if len(b.Messages) > 0 {
			msgid = b.Messages[len(b.Messages)-1].Tags["msgid"]
		}
		if msgid != "" && a.irc.IsConnected(b.Server) {
			if err := a.irc.MarkRead(b.Server, b.Target, msgid); err != nil {
				a.local(b.Server, b.Target, model.KindError, err.Error())
			}
		}
	case "caps":
		caps := a.irc.Capabilities(b.Server)
		a.local(b.Server, b.Target, model.KindSystem, "negotiated IRCv3: "+strings.Join(caps, ", "))
	case "dcc":
		a.executeDCC(b, arg1, tail)
	case "gotify":
		a.executeGotify(b, arg1)
	case "lua":
		a.executeLua(b, arg1, tail)
	case "close":
		if b.Target != "*server*" {
			a.closeBufferLocally(b.Server, b.Target)
			delete(a.transcriptCaches, model.Key(b.Server, b.Target))
			a.state.Close(b.Server, b.Target)
		}
	case "quit", "exit":
		a.irc.Stop(strings.TrimSpace(rest))
		a.stopped.Store(true)
	default:
		if a.scripts != nil {
			handled, err := a.scripts.Command(cmd, rest, scripting.Context{Server: b.Server, Target: b.Target, Nick: a.irc.CurrentNick(b.Server)})
			if err != nil {
				a.local(b.Server, b.Target, model.KindError, "Lua /"+cmd+": "+err.Error())
				return
			}
			if handled {
				return
			}
		}
		a.local(b.Server, b.Target, model.KindError, "unknown command /"+cmd+" — try /help")
	}
}

// resetUIAfterScriptReload forces the next render through a fresh transcript
// widget and requests a full physical-screen resync. A Lua reload can run
// startup hooks which print one or more lines before the reload command itself
// reports success. Rebuilding the List fixes gotui's private viewport state;
// forcing tcell.Sync after that rebuilt frame fixes stale terminal cells which
// an incremental render may otherwise leave behind.
func (a *App) resetUIAfterScriptReload() {
	a.transcriptReset = true
	a.forceScreenSync = true
	// executeLua normally runs inside the UI event loop, which renders after the
	// command returns. Keep this wake-up as a safety net for any future caller
	// that triggers a reload outside an input event.
	a.requestRedraw()
}

func (a *App) executeLua(b *model.Buffer, sub, tail string) {
	if a.scripts == nil {
		a.local(b.Server, b.Target, model.KindError, "Lua scripting is disabled")
		return
	}
	switch strings.ToLower(sub) {
	case "list":
		loaded := a.scripts.List()
		if len(loaded) == 0 {
			a.local(b.Server, b.Target, model.KindSystem, "Lua: no scripts loaded")
			return
		}
		a.local(b.Server, b.Target, model.KindSystem, "Lua scripts: "+strings.Join(loaded, ", "))
	case "reload":
		loaded, err := a.scripts.Reload()
		if err != nil {
			a.local(b.Server, b.Target, model.KindError, "Lua reload: "+err.Error())
			a.resetUIAfterScriptReload()
			return
		}
		a.local(b.Server, b.Target, model.KindSystem, fmt.Sprintf("Lua: loaded %d script(s)", len(loaded)))
		a.resetUIAfterScriptReload()
	case "eval":
		code := strings.TrimSpace(tail)
		if code == "" {
			a.local(b.Server, b.Target, model.KindError, "usage: /lua eval <code>")
			return
		}
		if err := a.scripts.Eval(code); err != nil {
			a.local(b.Server, b.Target, model.KindError, "Lua eval: "+err.Error())
		}
	default:
		a.local(b.Server, b.Target, model.KindSystem, "usage: /lua list | /lua reload | /lua eval <code>")
	}
}

func (a *App) executeGotify(b *model.Buffer, sub string) {
	if remote, ok := a.irc.(interface {
		GotifyStatus() (string, error)
		GotifyTest() error
	}); ok && (strings.EqualFold(sub, "status") || strings.EqualFold(sub, "test")) {
		server, target := b.Server, b.Target
		go func() {
			var text string
			var err error
			if strings.EqualFold(sub, "status") {
				text, err = remote.GotifyStatus()
			} else {
				err = remote.GotifyTest()
				text = "Relay Gotify test notification sent"
			}
			if err != nil {
				a.local(server, target, model.KindError, "Relay Gotify: "+err.Error())
				return
			}
			a.local(server, target, model.KindSystem, text)
		}()
		return
	}

	switch strings.ToLower(sub) {
	case "status":
		if a.gotify != nil && a.gotify.Enabled() {
			a.local(b.Server, b.Target, model.KindSystem, "Gotify: "+a.gotify.Status())
		} else {
			a.local(b.Server, b.Target, model.KindSystem, "Gotify disabled")
		}
	case "test":
		if a.gotify == nil || !a.gotify.Enabled() {
			a.local(b.Server, b.Target, model.KindError, "Gotify is disabled in configuration")
			return
		}
		server, target := b.Server, b.Target
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(a.cfg.Gotify.TimeoutSeconds)*time.Second)
			defer cancel()
			if err := a.gotify.Test(ctx); err != nil {
				a.local(server, target, model.KindError, "Gotify test failed: "+err.Error())
				return
			}
			a.local(server, target, model.KindSystem, "Gotify test notification sent")
		}()
	default:
		a.local(b.Server, b.Target, model.KindSystem, "usage: /gotify status | /gotify test")
	}
}

func (a *App) executeDCC(b *model.Buffer, sub, rest string) {
	sub = strings.ToLower(sub)
	peer, path := cutWord(rest)
	switch sub {
	case "list":
		offers := a.irc.DCCOffers()
		if len(offers) == 0 {
			a.local(b.Server, b.Target, model.KindDCC, "no pending offers")
			return
		}
		for _, o := range offers {
			a.local(b.Server, b.Target, model.KindDCC, fmt.Sprintf("%s %s from %s on %s", o.Kind, o.Filename, o.Peer, o.Server))
		}
	case "accept":
		if peer == "" {
			a.local(b.Server, b.Target, model.KindError, "usage: /dcc accept nick")
			return
		}
		err := a.irc.DCCAccept(b.Server, peer, func(line string) {
			a.local(b.Server, peer, model.KindDCC, "<"+peer+"> "+line)
		})
		if err != nil {
			a.local(b.Server, b.Target, model.KindError, err.Error())
		}
	case "send":
		if peer == "" || strings.TrimSpace(path) == "" {
			a.local(b.Server, b.Target, model.KindError, "usage: /dcc send nick /path/to/file")
			return
		}
		if err := a.irc.DCCSend(b.Server, peer, config.ExpandPath(strings.TrimSpace(path))); err != nil {
			a.local(b.Server, b.Target, model.KindError, err.Error())
		}
	default:
		a.local(b.Server, b.Target, model.KindDCC, "usage: /dcc list | /dcc accept nick | /dcc send nick path")
	}
}

// sendTextCommand is shared by interactive input and scripts. Interactive
// callers retain the complete command when delivery cannot be confirmed.
func (a *App) sendTextCommand(line string) (bool, error) {
	cmd, rest := cutWord(strings.TrimSpace(strings.TrimPrefix(line, "/")))
	cmd = strings.ToLower(cmd)
	if cmd != "msg" && cmd != "me" && cmd != "notice" {
		return false, nil
	}
	b := a.state.CurrentInfo()
	if b == nil {
		return true, fmt.Errorf("select a channel or query first")
	}
	arg, tail := cutWord(rest)
	switch cmd {
	case "msg":
		if arg == "" || strings.TrimSpace(tail) == "" {
			return true, fmt.Errorf("usage: /msg nick message")
		}
		a.reopenBuffer(b.Server, arg)
		a.state.Ensure(b.Server, arg)
		return true, a.irc.SendMessage(b.Server, arg, strings.TrimSpace(tail))
	case "me":
		return true, a.irc.SendAction(b.Server, b.Target, rest)
	default:
		if arg == "" || strings.TrimSpace(tail) == "" {
			return true, fmt.Errorf("usage: /notice target message")
		}
		return true, a.irc.Notice(b.Server, arg, strings.TrimSpace(tail))
	}
}

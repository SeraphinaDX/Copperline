package tui

import (
	"strings"
	"time"

	ircclient "copperline/internal/irc"
	"copperline/internal/model"
	"github.com/lrstanley/girc"
)

type pendingIRCQuery struct {
	server, target, command, subject string
	expires                          time.Time
	modeSeen                         bool
}

// Only track replies hidden as automatic channel housekeeping by the backend.
// Keeping this in the TUI also works through the relay's raw event stream.
func (a *App) trackIRCQuery(server, target string, event *girc.Event) *pendingIRCQuery {
	if event.Command != "NAMES" && event.Command != "WHO" && !(event.Command == "MODE" && len(event.Params) == 1 && model.IsChannel(event.Params[0])) {
		return nil
	}
	pending := &pendingIRCQuery{server: server, target: target, command: event.Command, subject: event.Params[0], expires: time.Now().Add(2 * time.Minute)}
	a.ircQueryMu.Lock()
	defer a.ircQueryMu.Unlock()
	a.pruneIRCQueries(nil, "")
	a.pendingIRCQueries = append(a.pendingIRCQueries, pending)
	return pending
}

func (a *App) forgetIRCQuery(pending *pendingIRCQuery) {
	a.ircQueryMu.Lock()
	defer a.ircQueryMu.Unlock()
	a.pruneIRCQueries(pending, "")
}

// Caller holds ircQueryMu. A disconnected server invalidates all its requests.
func (a *App) pruneIRCQueries(remove *pendingIRCQuery, server string) {
	now := time.Now()
	kept := a.pendingIRCQueries[:0]
	for _, p := range a.pendingIRCQueries {
		if p != remove && p.server != server && now.Before(p.expires) {
			kept = append(kept, p)
		}
	}
	clear(a.pendingIRCQueries[len(kept):])
	a.pendingIRCQueries = kept
}

func (a *App) handleIRCQueryReply(ev ircclient.Event) {
	if ev.Command == ircclient.EventDisconnected || ev.Command == ircclient.EventClosed {
		a.ircQueryMu.Lock()
		a.pruneIRCQueries(nil, ev.Server)
		a.ircQueryMu.Unlock()
		return
	}
	if len(ev.Params) < 2 {
		return
	}
	// The backend displays these errors. Stop tracking the failed request so
	// a later automatic refresh cannot be mistaken for its reply.
	switch ev.Command {
	case "401", "403", "442", "461", "263":
		a.ircQueryMu.Lock()
		a.pruneIRCQueries(nil, "")
		for _, p := range a.pendingIRCQueries {
			match := girc.ToRFC1459(p.subject) == girc.ToRFC1459(ev.Params[1])
			if ev.Command == "461" || ev.Command == "263" {
				match = strings.EqualFold(p.command, ev.Params[1])
			}
			if p.server == ev.Server && match {
				a.pruneIRCQueries(p, "")
				break
			}
		}
		a.ircQueryMu.Unlock()
		return
	}
	command, subject, text, end := "", ev.Params[1], "", false
	switch ev.Command {
	case "353":
		if len(ev.Params) < 4 {
			return
		}
		command, subject = "NAMES", ev.Params[2]
		text = "Names for " + subject + ": " + ev.Params[3]
	case "366":
		command, text, end = "NAMES", "End of NAMES for "+subject, true
	case "352", "354":
		if (ev.Command == "352" && len(ev.Params) < 8) || (ev.Command == "354" && len(ev.Params) < 3) {
			return
		}
		command, text = "WHO", "WHO: "+strings.Join(ev.Params[1:], " ")
	case "315":
		command, text, end = "WHO", "End of WHO for "+subject, true
	case "324", "329":
		if len(ev.Params) < 3 {
			return
		}
		command = "MODE"
		text = "Modes for " + subject + ": " + strings.Join(ev.Params[2:], " ")
		if ev.Command == "329" {
			text, end = "Channel created (Unix time): "+ev.Params[2], true
		}
	default:
		return
	}

	target := ""
	a.ircQueryMu.Lock()
	a.pruneIRCQueries(nil, "")
	for _, p := range a.pendingIRCQueries {
		if p.server != ev.Server || p.command != command {
			continue
		}
		// Standard WHO/WHOX rows do not echo their query mask. Match channel
		// WHO rows when possible; otherwise use the oldest outstanding request.
		matchSubject := command != "WHO" || end || (ev.Command == "352" && model.IsChannel(p.subject) && !strings.ContainsAny(p.subject, "*?"))
		if matchSubject && girc.ToRFC1459(p.subject) != girc.ToRFC1459(subject) {
			continue
		}
		if ev.Command == "324" && p.modeSeen {
			continue
		}
		if ev.Command == "329" && !p.modeSeen {
			continue
		}
		target = p.target
		if ev.Command == "324" {
			p.modeSeen = true // 329 is optional; expiry cleans up if it never arrives.
		}
		if end {
			a.pruneIRCQueries(p, "")
		}
		break
	}
	a.ircQueryMu.Unlock()
	if target != "" {
		a.local(ev.Server, target, model.KindSystem, text)
	}
}

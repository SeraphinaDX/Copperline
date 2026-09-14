package tui

import (
	"time"

	ircclient "copperline/internal/irc"
	"copperline/internal/model"
)

type pendingWhoisRequest struct {
	target  string
	expires time.Time
}

func (a *App) requestWhois(server, target, nick string) error {
	key := model.Key(server, nick)
	now := time.Now()
	a.whoisMu.Lock()
	if a.pendingWhois == nil {
		a.pendingWhois = make(map[string]pendingWhoisRequest)
	}
	for pendingKey, pending := range a.pendingWhois {
		if now.After(pending.expires) {
			delete(a.pendingWhois, pendingKey)
		}
	}
	a.pendingWhois[key] = pendingWhoisRequest{target: target, expires: now.Add(2 * time.Minute)}
	a.whoisMu.Unlock()

	if err := a.irc.Whois(server, nick); err != nil {
		a.whoisMu.Lock()
		delete(a.pendingWhois, key)
		a.whoisMu.Unlock()
		return err
	}
	a.local(server, target, model.KindSystem, "WHOIS requested for "+nick)
	return nil
}

func (a *App) handleWhoisReply(ev ircclient.Event) {
	reply, ok := ircclient.ParseWhoisReply(ev)
	if !ok {
		return
	}
	target := "*server*"
	key := model.Key(ev.Server, reply.Nick)
	now := time.Now()
	a.whoisMu.Lock()
	if pending, found := a.pendingWhois[key]; found {
		if now.Before(pending.expires) {
			target = pending.target
		} else {
			delete(a.pendingWhois, key)
		}
	}
	if reply.End {
		delete(a.pendingWhois, key)
	}
	a.whoisMu.Unlock()
	a.local(ev.Server, target, model.KindSystem, reply.Text)
}

package irc

import (
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/lrstanley/girc"
)

var sendSequence atomic.Uint64

// confirmSend fences the queued text with a unique IRC PING. A matching PONG
// proves the server read past the text on this connection; it does not prove
// that a recipient read it or that channel permissions allowed it. IRC errors
// still arrive through the normal event path. Never automatically retry an
// uncertain send: the text may have arrived even if the PONG was lost.
func confirmSend(c *girc.Client, send func(), timeout time.Duration) error {
	token := fmt.Sprintf("copperline-send-%d", sendSequence.Add(1))
	pong := make(chan struct{}, 1)
	disconnected := make(chan struct{}, 1)
	handler := c.Handlers.Add(girc.PONG, func(_ *girc.Client, e girc.Event) {
		if len(e.Params) > 0 && e.Params[len(e.Params)-1] == token {
			select {
			case pong <- struct{}{}:
			default:
			}
		}
	})
	defer c.Handlers.Remove(handler)
	handler = c.Handlers.Add(girc.DISCONNECTED, func(_ *girc.Client, _ girc.Event) {
		select {
		case disconnected <- struct{}{}:
		default:
		}
	})
	defer c.Handlers.Remove(handler)
	expired := make(chan struct{})
	timer := time.AfterFunc(timeout, func() {
		close(expired)
		c.Close()
	})
	defer timer.Stop()
	send()
	select {
	case <-expired:
		return errors.New("IRC send timed out; delivery is uncertain, check before retrying")
	case <-disconnected:
		return errors.New("IRC disconnected during send; delivery is uncertain, check before retrying")
	default:
	}
	c.Cmd.Ping(token)
	select {
	case <-pong:
		if !timer.Stop() {
			return errors.New("IRC confirmation arrived after timeout; check before retrying")
		}
		return nil
	case <-expired:
		return errors.New("IRC send timed out; delivery is uncertain, check before retrying")
	case <-disconnected:
		return errors.New("IRC disconnected during send; delivery is uncertain, check before retrying")
	}
}

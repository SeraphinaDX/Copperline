package clientcmd

import (
	"copperline/internal/irc"
	"errors"
	"strings"
	"testing"
)

type commandBackend struct {
	irc.Backend
	action, server, target, text string
	err                          error
}

func TestFlexUsesCurrentChannelOrQueryAndPropagatesSendErrors(t *testing.T) {
	for _, target := range []string{"#go", "Alice"} {
		for _, line := range []string{"/flex", "/Flex", "/FLEX"} {
			b := &commandBackend{err: errors.New("relay unavailable")}
			c := Context{Backend: b, Server: "Libera", Target: target}
			if err := Execute(c, line); !errors.Is(err, b.err) {
				t.Fatalf("failed flex send did not return the backend error: %v", err)
			}
			if b.action != "message" || b.server != "Libera" || b.target != target ||
				!strings.HasPrefix(b.text, "Copperline ") || !strings.Contains(b.text, "OS:") || !strings.Contains(b.text, "CPU:") {
				t.Fatalf("bad flex dispatch: %#v", b)
			}
		}
	}
}

func TestFlexValidatesBeforeSending(t *testing.T) {
	for _, tc := range []struct{ server, target, line string }{
		{"test", "*server*", "/flex"}, {"test", "", "/flex"}, {"", "#go", "/flex"},
		{"test", "#go", "/flex extra"}, {"test", "#go", "/flex\n"}, {"test", "#go", "/flex \x00"},
	} {
		b := &commandBackend{}
		handled, err := SendText(Context{Backend: b, Server: tc.server, Target: tc.target}, tc.line)
		if !handled || err == nil || b.action != "" {
			t.Fatalf("invalid flex was not rejected: %#v handled=%v err=%v", tc, handled, err)
		}
	}
}

func (b *commandBackend) SendMessage(s, t, text string) error {
	b.action, b.server, b.target, b.text = "message", s, t, text
	return b.err
}
func (b *commandBackend) SendAction(s, t, text string) error {
	b.action, b.server, b.target, b.text = "action", s, t, text
	return b.err
}
func (b *commandBackend) Notice(s, t, text string) error {
	b.action, b.server, b.target, b.text = "notice", s, t, text
	return b.err
}
func (b *commandBackend) Join(s, t, key string) error {
	b.action, b.server, b.target, b.text = "join", s, t, key
	return b.err
}
func (b *commandBackend) Part(s, t, text string) error {
	b.action, b.server, b.target, b.text = "part", s, t, text
	return b.err
}
func (b *commandBackend) Raw(s, line string) error {
	b.action, b.server, b.target, b.text = "raw", s, "", line
	return b.err
}

func TestCommandsKeepTargetsAndText(t *testing.T) {
	for _, tc := range []struct{ line, action, target, text string }{
		{"hello  there", "message", "#go", "hello  there"},
		{"/MSG Alice hello there", "message", "Alice", "hello there"},
		{"/me waves", "action", "#go", "waves"},
		{"/notice Alice hi", "notice", "Alice", "hi"},
		{"/join #new secret", "join", "#new", "secret"},
		{"/part goodbye everyone", "part", "#go", "goodbye everyone"},
		{"/part #new leaving now", "part", "#new", "leaving now"},
		{"/map", "raw", "", "MAP"},
	} {
		t.Run(tc.line, func(t *testing.T) {
			b := &commandBackend{}
			c := Context{Backend: b, Server: "Libera", Target: "#go"}
			if err := Execute(c, tc.line); err != nil {
				t.Fatal(err)
			}
			if b.action != tc.action || b.target != tc.target || b.text != tc.text || b.server != "Libera" {
				t.Fatalf("bad dispatch: %#v", b)
			}
		})
	}
}
func TestFailedSendIsNotRetried(t *testing.T) {
	failure := errors.New("delivery uncertain")
	b := &commandBackend{err: failure}
	if err := Execute(Context{Backend: b, Server: "test", Target: "#go"}, "message"); !errors.Is(err, failure) {
		t.Fatal(err)
	}
}
func TestInvalidInputNeverDispatches(t *testing.T) {
	for _, line := range []string{"/msg Alice", "/notice Alice", "/join nick", "/raw PRIVMSG #go :hi\nQUIT", "hello\x00there", "/bogus"} {
		b := &commandBackend{}
		if err := Execute(Context{Backend: b, Server: "test", Target: "*server*"}, line); err == nil {
			t.Fatalf("accepted %q", line)
		}
		if b.action != "" {
			t.Fatal("invalid input was sent")
		}
	}
}

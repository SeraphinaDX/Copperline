package irc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"copperline/internal/config"
	"copperline/internal/model"
)

func newIgnoreTestManager(t *testing.T) (*Manager, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ignores.toml")
	cfg := &config.Config{General: config.GeneralConfig{IgnoreFile: path}}
	return New(cfg, nil), path
}

func TestIgnoreRulesMatchIdentityAndScope(t *testing.T) {
	m, _ := newIgnoreTestManager(t)
	if _, err := m.ManageIgnore("libera", "#go", "add Alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ManageIgnore("libera", "#go", "add *!*@noisy.example --channel"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ManageIgnore("libera", "#go", "add re:^bot[0-9]+$ --global"); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		msg  model.Message
		want bool
	}{
		{"nick on current network", model.Message{Server: "libera", Target: "#elsewhere", Nick: "ALICE", Kind: model.KindMessage}, true},
		{"nick on another network", model.Message{Server: "oftc", Target: "#go", Nick: "Alice", Kind: model.KindMessage}, false},
		{"hostmask in scoped channel", model.Message{Server: "libera", Target: "#go", Nick: "someone", User: "u", Host: "noisy.example", Kind: model.KindAction}, true},
		{"hostmask outside scoped channel", model.Message{Server: "libera", Target: "#rust", Nick: "someone", User: "u", Host: "noisy.example", Kind: model.KindMessage}, false},
		{"global regular expression", model.Message{Server: "oftc", Target: "#go", Nick: "Bot42", Kind: model.KindNotice}, true},
		{"membership event remains visible", model.Message{Server: "libera", Target: "#go", Nick: "Alice", Text: "Alice joined", Kind: model.KindSystem}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := m.shouldIgnore(tt.msg); got != tt.want {
				t.Fatalf("shouldIgnore() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIgnoreRulesPersistAndCanBeRemoved(t *testing.T) {
	m, path := newIgnoreTestManager(t)
	if _, err := m.ManageIgnore("libera", "#go", "add Alice --channel"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("ignore file permissions = %o, want 600", info.Mode().Perm())
	}

	reloaded := New(&config.Config{General: config.GeneralConfig{IgnoreFile: path}}, nil)
	lines, err := reloaded.ManageIgnore("libera", "#go", "list")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(lines, "\n"); !strings.Contains(got, "Alice [libera / #go]") {
		t.Fatalf("reloaded list = %q", got)
	}
	if _, err := reloaded.ManageIgnore("libera", "#go", "remove 1"); err != nil {
		t.Fatal(err)
	}
	lines, err = reloaded.ManageIgnore("libera", "#go", "list")
	if err != nil || len(lines) != 1 || lines[0] != "ignore list is empty" {
		t.Fatalf("list after removal = %#v, %v", lines, err)
	}
}

func TestInvalidIgnoreFileIsNotOverwritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ignores.toml")
	if err := os.WriteFile(path, []byte("not valid = ["), 0o600); err != nil {
		t.Fatal(err)
	}
	m := New(&config.Config{General: config.GeneralConfig{IgnoreFile: path}}, nil)
	if _, err := m.ManageIgnore("libera", "#go", "add Alice"); err == nil {
		t.Fatal("corrupt ignore file did not block mutation")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "not valid = [" {
		t.Fatalf("corrupt ignore file was overwritten: %q", data)
	}
}

func TestEmitMessageDropsIgnoredChatBeforeSink(t *testing.T) {
	var delivered []model.Message
	m, _ := newIgnoreTestManager(t)
	m.SetMessageSink(func(msg model.Message) { delivered = append(delivered, msg) })
	if _, err := m.ManageIgnore("libera", "#go", "add Alice"); err != nil {
		t.Fatal(err)
	}
	m.emitMessage(model.Message{Server: "libera", Target: "#go", Nick: "Alice", Kind: model.KindMessage, Text: "hidden"})
	m.emitMessage(model.Message{Server: "libera", Target: "#go", Kind: model.KindSystem, Text: "Alice joined"})
	if len(delivered) != 1 || delivered[0].Text != "Alice joined" {
		t.Fatalf("delivered messages = %#v", delivered)
	}
}

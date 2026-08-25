package config

import "testing"

func TestCanonicalKeyBindingFriendlyNames(t *testing.T) {
	tests := map[string]string{
		"Alt+N":    "<M-n>",
		"Meta+P":   "<M-p>",
		"Ctrl+N":   "<C-n>",
		"F6":       "<F6>",
		"PageUp":   "<PageUp>",
		"PageDown": "<PageDown>",
		"Escape":   "<Escape>",
		"Tab":      "<Tab>",
	}
	for input, want := range tests {
		got, err := CanonicalKeyBinding(input)
		if err != nil {
			t.Fatalf("CanonicalKeyBinding(%q): %v", input, err)
		}
		if got != want {
			t.Fatalf("CanonicalKeyBinding(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestKeyBindingMatchesAltAliases(t *testing.T) {
	for _, eventID := range []string{"<M-n>", "<M-N>", "<A-n>", "<Alt-N>"} {
		if !KeyBindingMatches("Alt+N", eventID) {
			t.Fatalf("Alt+N did not match %q", eventID)
		}
	}
}

func TestApplyDefaultsAddsKeybindings(t *testing.T) {
	cfg := Config{}
	cfg.applyDefaults()
	if cfg.Keybindings.UserListDown != "Alt+N" || cfg.Keybindings.UserListUp != "Alt+P" {
		t.Fatalf("nick-list defaults = %q/%q, want Alt+N/Alt+P", cfg.Keybindings.UserListDown, cfg.Keybindings.UserListUp)
	}
	if cfg.Keybindings.NextBuffer != "Ctrl+N" || cfg.Keybindings.PreviousBuffer != "Ctrl+P" {
		t.Fatalf("buffer defaults = %q/%q, want Ctrl+N/Ctrl+P", cfg.Keybindings.NextBuffer, cfg.Keybindings.PreviousBuffer)
	}
}

func TestValidateRejectsConflictingKeybindings(t *testing.T) {
	cfg := Config{Servers: []ServerConfig{{Name: "test", Host: "irc.example.test"}}}
	cfg.applyDefaults()
	cfg.Keybindings.UserListDown = cfg.Keybindings.NextBuffer
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate accepted conflicting keybindings")
	}
}

func TestValidateRejectsInvalidKeybinding(t *testing.T) {
	cfg := Config{Servers: []ServerConfig{{Name: "test", Host: "irc.example.test"}}}
	cfg.applyDefaults()
	cfg.Keybindings.UserListDown = "Hyper+N"
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate accepted unsupported keybinding modifier")
	}
}

func TestValidateRejectsReservedInputKey(t *testing.T) {
	cfg := Config{Servers: []ServerConfig{{Name: "test", Host: "irc.example.test"}}}
	cfg.applyDefaults()
	cfg.Keybindings.UserListDown = "Enter"
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate accepted reserved Enter key for an action")
	}
}

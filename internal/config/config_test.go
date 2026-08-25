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

func TestHistoryDefaultsTakeUpAndDownAndMoveTranscriptLines(t *testing.T) {
	cfg := Config{}
	cfg.applyDefaults()
	if cfg.Keybindings.HistoryPrevious != "Up" || cfg.Keybindings.HistoryNext != "Down" {
		t.Fatalf("history defaults = %q/%q, want Up/Down", cfg.Keybindings.HistoryPrevious, cfg.Keybindings.HistoryNext)
	}
	if cfg.Keybindings.TranscriptLineUp != "Alt+K" || cfg.Keybindings.TranscriptLineDown != "Alt+J" {
		t.Fatalf("transcript line defaults = %q/%q, want Alt+K/Alt+J", cfg.Keybindings.TranscriptLineUp, cfg.Keybindings.TranscriptLineDown)
	}
}

func TestOldExplicitTranscriptArrowDefaultsAreMigrated(t *testing.T) {
	cfg := Config{Keybindings: KeybindingsConfig{TranscriptLineUp: "Up", TranscriptLineDown: "Down"}}
	cfg.applyDefaults()
	if cfg.Keybindings.HistoryPrevious != "Up" || cfg.Keybindings.HistoryNext != "Down" {
		t.Fatalf("history defaults = %q/%q, want Up/Down", cfg.Keybindings.HistoryPrevious, cfg.Keybindings.HistoryNext)
	}
	if cfg.Keybindings.TranscriptLineUp != "Alt+K" || cfg.Keybindings.TranscriptLineDown != "Alt+J" {
		t.Fatalf("old transcript arrows were not migrated: %q/%q", cfg.Keybindings.TranscriptLineUp, cfg.Keybindings.TranscriptLineDown)
	}
}

func TestInputHistoryLimitDefaultsToTen(t *testing.T) {
	cfg := Config{}
	cfg.applyDefaults()
	if got := cfg.General.InputHistoryLimitValue(); got != 10 {
		t.Fatalf("input history limit = %d, want 10", got)
	}
}

func TestInputHistoryLimitAllowsZeroAndRejectsNegative(t *testing.T) {
	zero := 0
	cfg := Config{
		General: GeneralConfig{InputHistoryLimit: &zero},
		Servers: []ServerConfig{{Name: "test", Host: "irc.example.test"}},
	}
	cfg.applyDefaults()
	if got := cfg.General.InputHistoryLimitValue(); got != 0 {
		t.Fatalf("input history limit = %d, want 0", got)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate rejected zero input history limit: %v", err)
	}

	negative := -1
	cfg.General.InputHistoryLimit = &negative
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate accepted negative input history limit")
	}
}

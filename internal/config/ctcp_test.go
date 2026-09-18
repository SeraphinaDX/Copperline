package config

import (
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestCTCPVersionConfig(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"", "Copperline by Britney Lozza"},
		{`ctcp_version = "Another IRC client 1.0"`, "Another IRC client 1.0"},
		{`ctcp_version = ""`, ""},
		{`ctcp_version = "  custom reply  "`, "custom reply"},
	} {
		var cfg Config
		if _, err := toml.Decode("[general]\n"+tc.input, &cfg); err != nil {
			t.Fatal(err)
		}
		cfg.applyDefaults()
		if got := cfg.General.CTCPVersionValue(); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestCTCPVersionRejectsProtocolDelimiters(t *testing.T) {
	for _, reply := range []string{"bad\rreply", "bad\nreply", "bad\x00reply", "bad\x01reply"} {
		cfg := Config{General: GeneralConfig{CTCPVersion: &reply}}
		cfg.applyDefaults()
		if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "ctcp_version") {
			t.Errorf("reply %q: expected CTCP config error, got %v", reply, err)
		}
	}
}

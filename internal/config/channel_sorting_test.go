package config

import "testing"

func TestChannelSortingConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name, setting string
		want          bool
	}{
		{"omitted", "", true},
		{"enabled", "sort_channels_alphabetically = true", true},
		{"disabled", "sort_channels_alphabetically = false", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := Decode("[general]\n" + tc.setting + "\n[[server]]\nname = 'test'\nhost = 'irc.example.test'\n")
			if err != nil {
				t.Fatal(err)
			}
			if got := cfg.General.SortChannelsAlphabeticallyEnabled(); got != tc.want {
				t.Fatalf("sorting = %v, want %v", got, tc.want)
			}
		})
	}
}

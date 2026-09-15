package tui

import (
	"strings"
	"testing"

	ui "github.com/metaspartan/gotui/v5"
)

func TestStyledPreservesBracketedIRCNicks(t *testing.T) {
	for _, nick := range []string{"[aruna]", "[aruna", "ar[un]a", "aruna]"} {
		t.Run(nick, func(t *testing.T) {
			cells := ui.ParseStyles(styled(nick, "#ff00ff"), ui.Style{})
			var visible strings.Builder
			for _, cell := range cells {
				if cell.Rune != '\u200b' {
					visible.WriteRune(cell.Rune)
				}
			}
			if got := visible.String(); got != nick {
				t.Fatalf("rendered nick = %q, want %q", got, nick)
			}
		})
	}
}

package model

import "testing"

func TestCustomHighlight(t *testing.T) {
	words := []string{"Copperline", "server down", "café", ""}
	for _, tc := range []struct {
		text string
		want bool
	}{
		{"COPPERLINE!", true}, {"CopperlineBot", false},
		{"the SERVER DOWN alert", true}, {"server downtime", false},
		{"un café?", true}, {"cafés", false}, {"écafé", false},
		{"hello MyNick!", true}, {"myNick2", false},
		{"\x0302Copperline\x0f", true}, {"nothing here", false},
	} {
		msg := Message{Nick: "alice", Text: tc.text, Kind: KindMessage}
		if got := Highlight(msg, "myNick", words); got != tc.want {
			t.Errorf("Highlight(%q) = %v, want %v", tc.text, got, tc.want)
		}
	}
	for _, kind := range []Kind{KindMessage, KindAction, KindNotice, KindSystem} {
		msg := Message{Nick: "alice", Text: "Copperline", Kind: kind}
		if got := Highlight(msg, "self", words); got != (kind == KindMessage || kind == KindAction) {
			t.Errorf("unexpected eligibility for %s", kind)
		}
		if Highlight(msg, "Alice", words) {
			t.Fatal("own message triggered highlight")
		}
	}
}

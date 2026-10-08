package tui

import (
	"reflect"
	"testing"

	"copperline/internal/model"
)

func TestChannelSortingDisplayAndNavigation(t *testing.T) {
	for _, sorted := range []bool{true, false} {
		name := "default"
		if !sorted {
			name = "disabled"
		}
		t.Run(name, func(t *testing.T) {
			a := newCatchupApp(t)
			a.cfg.General.SortChannelsAlphabetically = nil
			if !sorted {
				a.cfg.General.SortChannelsAlphabetically = &sorted
			}
			a.state = model.New(100)
			servers := a.irc.ServerNames()
			server := servers[0]
			for _, s := range servers {
				a.state.Ensure(s, "*server*")
			}
			for _, target := range []string{"#zebra", "#Beta", "#alpha"} {
				a.state.Ensure(server, target)
			}
			a.selectBuffer(server, "#zebra")
			a.rebuildSidebar()
			want := []string{model.Key(server, "*server*"), model.Key(server, "#alpha"), model.Key(server, "#Beta"), model.Key(server, "#zebra")}
			if !sorted {
				want[1], want[3] = want[3], want[1]
			}
			for _, s := range servers[1:] {
				want = append(want, model.Key(s, "*server*"))
			}
			keys, _ := a.sidebarOrder()
			if !reflect.DeepEqual(a.sidebarKeys, want) || !reflect.DeepEqual(keys, want) {
				t.Fatalf("display = %v, navigation = %v, want %v", a.sidebarKeys, keys, want)
			}
			if a.sidebarKeys[a.sidebar.SelectedRow] != model.Key(server, "#zebra") {
				t.Fatal("sorting moved selected buffer")
			}
			a.selectBufferNumber(2)
			if a.state.CurrentKey != want[1] {
				t.Fatal("numbered jump disagrees with sidebar")
			}
			a.selectRelative(1)
			if a.state.CurrentKey != want[2] {
				t.Fatal("next buffer disagrees with sidebar")
			}
			a.selectRelative(-1)
			if a.state.CurrentKey != want[1] {
				t.Fatal("previous buffer disagrees with sidebar")
			}
			// An earlier channel must not move selection to a different target.
			a.state.Ensure(server, "#aardvark")
			a.rebuildSidebar()
			if a.sidebarKeys[a.sidebar.SelectedRow] != want[1] {
				t.Fatal("new channel changed selection")
			}
			a.selectBuffer(server, "*server*")
			for _, target := range []string{"#zebra", "#Beta", "#alpha"} {
				addCatchupMessages(a, server, target, "unread")
			}
			a.selectNextUnread()
			if a.state.CurrentKey != want[1] {
				t.Fatal("next unread disagrees with sidebar")
			}
		})
	}
}

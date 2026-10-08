package gui

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"
)

func TestChannelSortingDesktopAndMobile(t *testing.T) {
	for _, mobile := range []bool{false, true} {
		for _, sorted := range []bool{true, false} {
			t.Run(fmt.Sprintf("mobile=%v/sorted=%v", mobile, sorted), func(t *testing.T) {
				t.Setenv("TMPDIR", t.TempDir())
				a := test.NewApp()
				defer a.Quit()
				profile := strings.Replace(starterConfig, `host_key_fingerprint = ""`, `host_key_fingerprint = "SHA256:test"`, 1)
				if sorted {
					profile = strings.Replace(profile, "sort_channels_alphabetically = true\n", "", 1)
				} else {
					profile = strings.Replace(profile, "sort_channels_alphabetically = true", "sort_channels_alphabetically = false", 1)
				}
				if err := (Storage{App: a}).SaveConfig(profile); err != nil {
					t.Fatal(err)
				}
				g, err := New(a, "")
				if err != nil {
					t.Fatal(err)
				}
				defer g.shutdown()
				g.mobile = mobile
				g.build()
				for _, target := range []string{"*server*", "#zebra", "#Beta", "#alpha"} {
					g.state.Ensure("test", target)
				}
				g.state.Select("test", "#zebra")
				g.refresh()
				want := []string{"*server*", "#alpha", "#Beta", "#zebra"}
				if !sorted {
					want[1], want[3] = want[3], want[1]
				}
				var got []string
				for _, b := range g.buffers {
					got = append(got, b.Target)
				}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("channel list = %v, want %v", got, want)
				}
				g.entry.SetText("draft for zebra")
				g.state.Ensure("test", "#aardvark")
				g.refresh()
				if g.state.CurrentInfo().Target != "#zebra" || g.entry.Text != "draft for zebra" {
					t.Fatal("new channel changed current buffer or draft")
				}
				// Both the desktop list and mobile panel call this callback.
				g.channels.OnSelected(1)
				selected := "#zebra"
				if sorted {
					selected = "#aardvark"
				}
				if g.state.CurrentInfo().Target != selected {
					t.Fatal("channel click selected a different target than displayed")
				}
				g.selectBuffer("test", "#zebra")
				if g.entry.Text != "draft for zebra" {
					t.Fatal("sorting lost buffer draft")
				}
			})
		}
	}
}

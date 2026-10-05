package gui

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"copperline/internal/model"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

func TestIdentityAndConfigSurviveRestart(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	a := test.NewApp()
	defer a.Quit()
	s := Storage{App: a}
	first, err := s.Signer()
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Signer()
	if err != nil {
		t.Fatal(err)
	}
	if string(first.PublicKey().Marshal()) != string(second.PublicKey().Marshal()) {
		t.Fatal("identity changed")
	}
	if err = s.SaveConfig(starterConfig); err != nil {
		t.Fatal(err)
	}
	changed := starterConfig + "\n# saved again\n"
	if err = s.SaveConfig(changed); err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadConfig()
	if err != nil || got != changed {
		t.Fatalf("config=%q err=%v", got, err)
	}
	// Corrupt existing keys must produce an error, not a replacement identity.
	w, err := a.Storage().Save("relay_ed25519.pem")
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte("broken key"))
	w.Close()
	if _, err = s.Signer(); err == nil {
		t.Fatal("corrupt identity replaced")
	}
}

func TestDesktopConfigIsNotModifiedByLoading(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	a := test.NewApp()
	defer a.Quit()
	path := filepath.Join(t.TempDir(), "client.toml")
	os.WriteFile(path, []byte(starterConfig), 0600)
	s := Storage{App: a, ConfigPath: path}
	if _, err := s.LoadConfig(); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveConfig(starterConfig + "# comment\n"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatal("config permissions changed")
	}
}

func TestPhoneAndDesktopLayoutAndDrafts(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	a := test.NewApp()
	defer a.Quit()
	g, err := New(a, "")
	if err != nil {
		t.Fatal(err)
	}
	defer g.shutdown()
	// Close the initial settings overlay so screenshots and bounds cover chat.
	overlays := g.window.Canvas().Overlays()
	for _, o := range overlays.List() {
		overlays.Remove(o)
	}
	g.state.Ensure("Libera", "*server*")
	g.state.Ensure("Libera", "#go")
	g.state.Ensure("Libera", "#linux")
	g.state.Select("Libera", "#go")
	for i, line := range []string{"Welcome to Copperline.", "The same relay client runs on desktop and Android.", "Long messages wrap to the available width instead of pushing the composer off the phone screen."} {
		g.state.Add(model.Message{RelayID: line, Time: time.Date(2026, 10, 5, 9, 14+i, 0, 0, time.UTC), Server: "Libera", Target: "#go", Nick: "alice", Text: line, Kind: model.KindMessage})
	}
	g.refresh()
	g.entry.SetText("draft for Go")
	g.selectBuffer("Libera", "#linux")
	g.entry.SetText("draft for Linux")
	g.selectBuffer("Libera", "#go")
	if g.entry.Text != "draft for Go" {
		t.Fatal("draft was lost on buffer change")
	}
	g.users = []string{"@Britney", "+alice", "bob"}
	g.nicklist.Refresh()
	for _, size := range []fyne.Size{fyne.NewSize(1100, 720), fyne.NewSize(360, 720), fyne.NewSize(320, 568)} {
		g.window.Resize(size)
		g.root.Layout.Layout(g.root.Objects, g.root.Size())
		if g.channelPane.Visible() != (size.Width >= 800) || g.userPane.Visible() != (size.Width >= 800) {
			t.Fatal("sidebar visibility")
		}
		if g.entry.Position().X+g.entry.Size().Width > g.chat.Size().Width+1 {
			t.Fatal("composer exceeds chat width")
		}
		if size.Width < 800 && g.chat.Size().Width != g.root.Size().Width {
			t.Fatal("chat did not fill phone")
		}
		if out := os.Getenv("COPPERLINE_GUI_PREVIEW_DIR"); out != "" {
			os.MkdirAll(out, 0755)
			name := "desktop.png"
			if size.Width < 800 {
				name = "phone.png"
			}
			f, err := os.Create(filepath.Join(out, name))
			if err != nil {
				t.Fatal(err)
			}
			if err = png.Encode(f, g.window.Canvas().Capture()); err != nil {
				t.Fatal(err)
			}
			f.Close()
		}
	}
}

func TestGUIRejectsUnverifiedOrDirectConnections(t *testing.T) {
	for _, data := range []string{starterConfig, `[relay]
mode="client"
address="localhost:2222"
insecure_skip_host_key_check=true
`, `[relay]
mode="direct"
[[server]]
name="test"
host="localhost"
`} {
		if err := validateGUIConfig(data); err == nil {
			t.Fatal("invalid GUI profile accepted")
		}
	}
}

func TestFailedSendPreservesNewDraftAndRelayProfile(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	a := test.NewApp()
	defer a.Quit()
	g, err := New(a, "")
	if err != nil {
		t.Fatal(err)
	}
	defer g.shutdown()
	g.state.Select("test", "#go")
	g.profileID = "old-relay"
	key := model.Key("test", "#go")
	if !g.restoreDraft("old-relay", g.drafts, key, "unsent") {
		t.Fatal("did not restore draft")
	}
	if g.entry.Text != "unsent" {
		t.Fatal("unsent text missing")
	}
	g.entry.SetText("newer text")
	if g.restoreDraft("old-relay", g.drafts, key, "failed text") {
		t.Fatal("active typing must offer unsent text separately")
	}
	if g.entry.Text != "newer text" {
		t.Fatal("newer typing overwritten")
	}
	g.selectBuffer("test", "#linux")
	if g.restoreDraft("old-relay", g.drafts, key, "failed text") {
		t.Fatal("saved newer draft overwritten")
	}
	other := model.Key("test", "#other")
	if !g.restoreDraft("old-relay", g.drafts, other, "failed text") {
		t.Fatal("empty inactive draft not restored")
	}
	if g.drafts[other] != "failed text" {
		t.Fatal("failed send missing")
	}
	if g.restoreDraft("old-relay", g.drafts, other, "another failure") {
		t.Fatal("occupied draft overwritten")
	}

	oldDrafts := map[string]string{}
	g.profileID = "new-relay"
	g.entry.SetText("")
	if !g.restoreDraft("old-relay", oldDrafts, key, "old destination") {
		t.Fatal("old draft not retained")
	}
	if g.entry.Text != "" || oldDrafts[key] != "old destination" {
		t.Fatal("draft crossed relay profiles")
	}
}

func TestPhoneSetupFitsScreen(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	a := test.NewApp()
	defer a.Quit()
	g, err := New(a, "")
	if err != nil {
		t.Fatal(err)
	}
	defer g.shutdown()
	g.window.Resize(fyne.NewSize(360, 640))
	if out := os.Getenv("COPPERLINE_GUI_PREVIEW_DIR"); out != "" {
		f, err := os.Create(filepath.Join(out, "phone-setup.png"))
		if err != nil {
			t.Fatal(err)
		}
		if err = png.Encode(f, g.window.Canvas().Capture()); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
}


func TestSecondaryLabelDispatchesRightClick(t *testing.T) {
	item := newSecondaryLabel()
	called := false
	item.onSecondary = func(ev *fyne.PointEvent) {
		called = ev != nil && ev.AbsolutePosition == fyne.NewPos(12, 34)
	}
	item.TappedSecondary(&fyne.PointEvent{AbsolutePosition: fyne.NewPos(12, 34)})
	if !called {
		t.Fatal("secondary tap was not dispatched")
	}
}

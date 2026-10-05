package gui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

func TestDesktopTabCompletesAndCyclesNicknames(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	a := test.NewApp()
	defer a.Quit()

	g, err := New(a, "")
	if err != nil {
		t.Fatal(err)
	}
	defer g.shutdown()

	g.state.Ensure("Libera", "#go")
	g.state.Select("Libera", "#go")
	g.users = []string{"@Britney", "+brad", "alice"}

	g.entry.SetText("br")
	g.entry.CursorRow = 0
	g.entry.CursorColumn = 2
	g.entry.TypedKey(&fyne.KeyEvent{Name: fyne.KeyTab})
	if g.entry.Text != "Britney: " {
		t.Fatalf("first completion = %q, want %q", g.entry.Text, "Britney: ")
	}

	g.entry.TypedKey(&fyne.KeyEvent{Name: fyne.KeyTab})
	if g.entry.Text != "brad: " {
		t.Fatalf("cycled completion = %q, want %q", g.entry.Text, "brad: ")
	}
}

func TestDesktopTabCompletesNickInMiddleOfMessage(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	a := test.NewApp()
	defer a.Quit()

	g, err := New(a, "")
	if err != nil {
		t.Fatal(err)
	}
	defer g.shutdown()

	g.state.Ensure("Libera", "#go")
	g.state.Select("Libera", "#go")
	g.users = []string{"@Britney", "alice"}

	g.entry.SetText("hello Bri")
	g.entry.CursorRow = 0
	g.entry.CursorColumn = len([]rune(g.entry.Text))
	g.entry.TypedKey(&fyne.KeyEvent{Name: fyne.KeyTab})
	if g.entry.Text != "hello Britney" {
		t.Fatalf("middle completion = %q", g.entry.Text)
	}
}

func TestMobileComposerDoesNotCaptureTab(t *testing.T) {
	e := newComposerEntry(false, func() bool {
		t.Fatal("mobile composer should not invoke nickname completion")
		return true
	}, nil)
	if e.AcceptsTab() {
		t.Fatal("mobile composer unexpectedly captures Tab")
	}
}

func TestCompletionRequiresChannel(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	a := test.NewApp()
	defer a.Quit()

	g, err := New(a, "")
	if err != nil {
		t.Fatal(err)
	}
	defer g.shutdown()

	g.state.Ensure("Libera", "alice")
	g.state.Select("Libera", "alice")
	g.users = []string{"alice"}
	g.entry.SetText("al")
	g.entry.CursorColumn = 2
	if g.completeNick() {
		t.Fatal("nickname completion should not run in a query")
	}
	if g.entry.Text != "al" {
		t.Fatalf("query text changed: %q", g.entry.Text)
	}

}

package version

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestFyneMetadataMatchesApplicationVersion(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate version test")
	}
	path := filepath.Join(filepath.Dir(file), "..", "..", "cmd", "copperline-gui", "FyneApp.toml")

	var app struct {
		Details struct {
			Version string
			Build   int
			ID      string
		}
	}
	if _, err := toml.DecodeFile(path, &app); err != nil {
		t.Fatal(err)
	}
	if app.Details.Version != Current {
		t.Fatalf("Fyne version = %q, application version = %q", app.Details.Version, Current)
	}
	if app.Details.Build != Build {
		t.Fatalf("Fyne build = %d, application build = %d", app.Details.Build, Build)
	}
	const appID = "ca.cerberusgames.copperline"
	if app.Details.ID != appID {
		t.Fatalf("Fyne app ID = %q, want %q", app.Details.ID, appID)
	}

	desktopPath := filepath.Join(filepath.Dir(file), "..", "..", "packaging", "linux", appID+".desktop")
	desktop, err := os.ReadFile(desktopPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(desktop)
	if !strings.Contains(text, "Icon="+appID) {
		t.Fatal("Linux desktop icon name does not match the Wayland app ID")
	}
	if !strings.Contains(text, "Exec=copperline-gui") {
		t.Fatal("Linux desktop entry does not launch copperline-gui")
	}
}

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
}

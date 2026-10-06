package version

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
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

	manifestPath := filepath.Join(filepath.Dir(path), "AndroidManifest.xml")
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var android struct {
		Package     string `xml:"package,attr"`
		Version     string `xml:"versionName,attr"`
		Build       string `xml:"versionCode,attr"`
		Application struct {
			Icon     string `xml:"icon,attr"`
			Activity struct {
				SoftInputMode string `xml:"windowSoftInputMode,attr"`
				Metadata      struct {
					Name  string `xml:"name,attr"`
					Value string `xml:"value,attr"`
				} `xml:"meta-data"`
			} `xml:"activity"`
		} `xml:"application"`
	}
	if err = xml.Unmarshal(manifest, &android); err != nil {
		t.Fatal(err)
	}
	if android.Package != appID || android.Version != Current || android.Build != strconv.Itoa(Build) {
		t.Fatalf("Android manifest metadata differs from Fyne/app metadata: %s %s/%s", android.Package, android.Version, android.Build)
	}
	if android.Application.Activity.SoftInputMode != "adjustResize" {
		t.Fatal("Android must resize the chat viewport above the keyboard")
	}
	if android.Application.Icon != "@mipmap/ic_launcher" {
		t.Fatal("Android manifest must retain the adaptive launcher icon")
	}
	metadata := android.Application.Activity.Metadata
	if metadata.Name != "android.app.lib_name" || metadata.Value != "Copperline" {
		t.Fatal("Android native library must match the Fyne package name")
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

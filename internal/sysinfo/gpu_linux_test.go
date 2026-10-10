//go:build linux && !android

package sysinfo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPCIGPUFallbackFiltersDevicesAndPreservesIDs(t *testing.T) {
	root := t.TempDir()
	for i, sample := range []struct{ class, vendor, device string }{
		{"0x030000", "0x8086", "0x5917"},
		{"0x030200", "0x10de", "0x2882"},
		{"0x020000", "0x8086", "0x1234"},
		{"invalid", "0x1002", "0x1234"},
		{"0x030000", "bad", "bad"},
	} {
		dir := filepath.Join(root, string(rune('a'+i)))
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		for name, value := range map[string]string{"class": sample.class, "vendor": sample.vendor, "device": sample.device} {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(value+"\n"), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	want := "Intel GPU (8086:5917); NVIDIA GPU (10de:2882)"
	if got := pciGPUs(root); got != want {
		t.Fatalf("sysfs GPU fallback = %q, want %q", got, want)
	}
	if got := pciGPUs(filepath.Join(root, "missing")); got != "" {
		t.Fatalf("missing sysfs produced GPU: %q", got)
	}
}

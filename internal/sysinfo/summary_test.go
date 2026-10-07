package sysinfo

import (
	"strings"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	"copperline/internal/version"
)

func TestSummaryFormatsSystemStats(t *testing.T) {
	s := snapshot{os: "Example Linux", kernel: "Linux 6.18", arch: "amd64", cpu: "Example CPU", cores: 8,
		totalRAM: 16 << 30, availableRAM: 6 << 30, memoryUsageKnown: true,
		uptime: 49*time.Hour + 3*time.Minute, uptimeKnown: true, load: "0.20 0.30 0.40"}
	want := "Copperline " + version.Current + " | OS: Example Linux (Linux 6.18)/amd64 | CPU: Example CPU — 8 logical CPUs | RAM: 10.0/16.0 GiB used | Up: 2d 1h 3m | Load: 0.20 0.30 0.40"
	if got := s.format(); got != want {
		t.Fatalf("summary = %q, want %q", got, want)
	}
}

func TestSummaryOmitsUnavailableStats(t *testing.T) {
	s := snapshot{os: "Android", arch: "arm64", cores: 8}
	text := s.format()
	for _, unavailable := range []string{"RAM:", "Up:", "Load:", "Device:"} {
		if strings.Contains(text, unavailable) {
			t.Fatalf("unknown statistic fabricated: %q", text)
		}
	}
	s.totalRAM = 8 << 30
	s.availableRAM = 9 << 30
	s.memoryUsageKnown = true
	if text = s.format(); !strings.Contains(text, "RAM: 8.0 GiB total") || strings.Contains(text, "GiB used") {
		t.Fatalf("invalid memory sample must not underflow: %q", text)
	}
}

func TestSummaryBoundsAndSanitizesPlatformData(t *testing.T) {
	s := snapshot{os: strings.Repeat("界", 100), kernel: strings.Repeat("X", 100), arch: "arm64",
		cpu: "CPU\r\n\x01ACTION\x01\x00\x1b[31m" + strings.Repeat("界", 100), cores: 128,
		device: strings.Repeat("界", 100), totalRAM: 16 << 30, uptime: time.Hour, uptimeKnown: true, load: "1 2 3"}
	text := s.format()
	if len(text) > 350 || !utf8.ValidString(text) {
		t.Fatalf("invalid IRC summary: %d bytes %q", len(text), text)
	}
	for _, r := range text {
		if unicode.IsControl(r) {
			t.Fatalf("control character in summary: %q", text)
		}
	}
}

func TestUptimeUsesSystemDuration(t *testing.T) {
	for _, tc := range []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "0m"}, {90 * time.Minute, "1h 30m"}, {24 * time.Hour, "1d 0h 0m"},
	} {
		if got := formatUptime(tc.d); got != tc.want {
			t.Fatalf("uptime %v = %q, want %q", tc.d, got, tc.want)
		}
	}
}

func TestLiveSummaryHasPortableBaseline(t *testing.T) {
	text := Summary()
	if !strings.Contains(text, "OS:") || !strings.Contains(text, "logical CPUs") || len(text) > 350 || !utf8.ValidString(text) {
		t.Fatalf("invalid local summary: %q", text)
	}
	t.Log(text)
}

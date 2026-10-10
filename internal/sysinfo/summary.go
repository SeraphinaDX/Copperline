// Package sysinfo collects a small, best-effort snapshot of the local device for
// /flex. It never queries the IRC relay, network services, or identifying data.
package sysinfo

import (
	"fmt"
	"runtime"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"copperline/internal/version"
)

type snapshot struct {
	os, kernel, arch, cpu, gpu, device string
	cores                              int
	totalRAM, availableRAM             uint64
	memoryUsageKnown                   bool
	uptime                             time.Duration
	uptimeKnown                        bool
}

// Summary reports this client's device, even when messages go through a relay.
// Missing or restricted system APIs simply leave optional fields out.
func Summary() string {
	s := snapshot{os: runtime.GOOS, arch: runtime.GOARCH, cores: runtime.NumCPU()}
	collect(&s)
	return s.format()
}

func (s snapshot) format() string {
	osName := clean(s.os, 48)
	if kernel := clean(s.kernel, 40); kernel != "" {
		osName += " (" + kernel + ")"
	}
	parts := []string{"Copperline " + version.Current, "OS: " + osName + "/" + clean(s.arch, 12)}
	if device := clean(s.device, 40); device != "" {
		parts = append(parts, "Device: "+device)
	}
	cpu := clean(s.cpu, 64)
	if cpu != "" {
		cpu += " — "
	}
	parts = append(parts, fmt.Sprintf("CPU: %s%d logical CPUs", cpu, s.cores))
	if gpu := clean(s.gpu, 112); gpu != "" {
		parts = append(parts, "GPU: "+gpu)
	}
	if s.totalRAM > 0 {
		const gib = float64(1 << 30)
		if s.memoryUsageKnown && s.availableRAM <= s.totalRAM {
			parts = append(parts, fmt.Sprintf("RAM: %.1f/%.1f GiB used", float64(s.totalRAM-s.availableRAM)/gib, float64(s.totalRAM)/gib))
		} else {
			parts = append(parts, fmt.Sprintf("RAM: %.1f GiB total", float64(s.totalRAM)/gib))
		}
	}
	if s.uptimeKnown && s.uptime >= 0 {
		parts = append(parts, "Up: "+formatUptime(s.uptime))
	}
	// Leave room for IRC command, target, and server prefix overhead. Truncate
	// on a UTF-8 boundary so long platform names cannot produce broken text.
	return clean(strings.Join(parts, " | "), 350)
}

func formatUptime(d time.Duration) string {
	minutes := int64(d / time.Minute)
	if minutes >= 1440 {
		return fmt.Sprintf("%dd %dh %dm", minutes/1440, minutes/60%24, minutes%60)
	}
	if minutes >= 60 {
		return fmt.Sprintf("%dh %dm", minutes/60, minutes%60)
	}
	return fmt.Sprintf("%dm", minutes)
}

func clean(value string, limit int) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return ' '
		}
		if unicode.IsControl(r) || r == utf8.RuneError {
			return -1
		}
		return r
	}, value)
	value = strings.Join(strings.Fields(value), " ")
	if len(value) <= limit {
		return value
	}
	end := limit - 3
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return strings.TrimSpace(value[:end]) + "..."
}

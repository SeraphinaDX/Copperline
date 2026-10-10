//go:build linux || android

package sysinfo

import (
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

func collect(s *snapshot) {
	platform(s)
	var uname unix.Utsname
	if unix.Uname(&uname) == nil {
		var release []byte
		for _, c := range uname.Release {
			if c == 0 {
				break
			}
			release = append(release, byte(c))
		}
		s.kernel = "Linux " + string(release)
	}
	var info unix.Sysinfo_t
	if unix.Sysinfo(&info) == nil {
		unit := uint64(info.Unit)
		if unit == 0 {
			unit = 1
		}
		s.totalRAM = uint64(info.Totalram) * unit
		s.uptime, s.uptimeKnown = time.Duration(info.Uptime)*time.Second, info.Uptime >= 0
	}
	s.cpu = cpuModel(readSystemFile("/proc/cpuinfo"))
	total, available, known := memoryInfo(readSystemFile("/proc/meminfo"))
	if total > 0 {
		s.totalRAM, s.availableRAM, s.memoryUsageKnown = total, available, known
	}
}

// Kernel files and os-release are bounded reads, not shell commands.
func readSystemFile(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	b, _ := io.ReadAll(io.LimitReader(f, 64*1024))
	return string(b)
}

func cpuModel(text string) string {
	hardware, processor := "", ""
	for _, line := range strings.Split(text, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.TrimSpace(key) {
		case "model name":
			if value != "" {
				return value
			}
		case "Hardware":
			hardware = value
		case "Processor":
			processor = value
		}
	}
	if hardware != "" {
		return hardware
	}
	return processor
}

func memoryInfo(text string) (total, available uint64, known bool) {
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 3 || fields[2] != "kB" {
			continue
		}
		n, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil || n > ^uint64(0)/1024 {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			total = n * 1024
		case "MemAvailable:":
			available, known = n*1024, true
		}
	}
	return total, available, known && total > 0 && available <= total
}

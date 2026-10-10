//go:build darwin && !ios

package sysinfo

import (
	"time"

	"golang.org/x/sys/unix"
)

func collect(s *snapshot) {
	s.os = "macOS"
	s.gpu = profilerGPUs(commandOutput(1500*time.Millisecond, "/usr/sbin/system_profiler", "SPDisplaysDataType", "-json", "-detailLevel", "mini"))
	if release, err := unix.Sysctl("kern.osproductversion"); err == nil {
		s.os += " " + release
	}
	if kernel, err := unix.Sysctl("kern.osrelease"); err == nil {
		s.kernel = "Darwin " + kernel
	}
	s.cpu, _ = unix.Sysctl("machdep.cpu.brand_string")
	s.totalRAM, _ = unix.SysctlUint64("hw.memsize")
	if boot, err := unix.SysctlTimeval("kern.boottime"); err == nil {
		s.uptime = time.Since(time.Unix(boot.Sec, int64(boot.Usec)*1000))
		s.uptimeKnown = s.uptime >= 0
	}
}

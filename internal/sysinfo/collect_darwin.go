//go:build darwin && !ios

package sysinfo

import (
	"encoding/binary"
	"fmt"
	"time"

	"golang.org/x/sys/unix"
)

func collect(s *snapshot) {
	s.os = "macOS"
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
	// Both supported Darwin architectures use three uint32 values, four bytes
	// of alignment padding, then the 64-bit long containing their scale.
	if data, err := unix.SysctlRaw("vm.loadavg"); err == nil && len(data) >= 24 {
		scale := float64(binary.NativeEndian.Uint64(data[16:24]))
		if scale > 0 {
			s.load = fmt.Sprintf("%.2f %.2f %.2f", float64(binary.NativeEndian.Uint32(data[0:4]))/scale,
				float64(binary.NativeEndian.Uint32(data[4:8]))/scale, float64(binary.NativeEndian.Uint32(data[8:12]))/scale)
		}
	}
}

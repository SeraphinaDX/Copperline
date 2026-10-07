package sysinfo

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// MEMORYSTATUSEX uses fixed-width fields on both 32-bit and 64-bit Windows.
type memoryStatus struct {
	length, load                                                                                         uint32
	totalPhys, availPhys, totalPageFile, availPageFile, totalVirtual, availVirtual, availExtendedVirtual uint64
}

func collect(s *snapshot) {
	v := windows.RtlGetVersion()
	s.os = fmt.Sprintf("Windows %d.%d build %d", v.MajorVersion, v.MinorVersion, v.BuildNumber)
	s.uptime, s.uptimeKnown = windows.DurationSinceBoot(), true
	if key, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DESCRIPTION\System\CentralProcessor\0`, registry.QUERY_VALUE); err == nil {
		s.cpu, _, _ = key.GetStringValue("ProcessorNameString")
		key.Close()
	}
	proc := windows.NewLazySystemDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx")
	if proc.Find() != nil {
		return
	}
	var memory memoryStatus
	memory.length = uint32(unsafe.Sizeof(memory))
	if result, _, _ := proc.Call(uintptr(unsafe.Pointer(&memory))); result != 0 {
		s.totalRAM, s.availableRAM, s.memoryUsageKnown = memory.totalPhys, memory.availPhys, true
	}
}

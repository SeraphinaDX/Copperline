package sysinfo

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// DISPLAY_DEVICEW uses fixed-width WCHAR/DWORD fields on all Windows targets.
type displayDevice struct {
	size        uint32
	name        [32]uint16
	description [128]uint16
	flags       uint32
	id, key     [128]uint16
}

func windowsGPUs() string {
	proc := windows.NewLazySystemDLL("user32.dll").NewProc("EnumDisplayDevicesW")
	if proc.Find() != nil {
		return ""
	}
	var names []string
	for i := uint32(0); i < 64; i++ {
		var device displayDevice
		device.size = uint32(unsafe.Sizeof(device))
		if result, _, _ := proc.Call(0, uintptr(i), uintptr(unsafe.Pointer(&device)), 0); result == 0 {
			break
		}
		// Exclude mirroring drivers and remote-session display adapters.
		if device.flags&(0x00000008|0x04000000) == 0 {
			names = append(names, windows.UTF16ToString(device.description[:]))
		}
	}
	return gpuNames(names)
}

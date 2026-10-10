//go:build linux && !android

package sysinfo

import (
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func linuxGPUs() string {
	if names := lspciGPUs(commandOutput(750*time.Millisecond, "lspci", "-vmm", "-nn")); names != "" {
		return names
	}
	return pciGPUs("/sys/bus/pci/devices")
}

// When pciutils is absent, PCI sysfs still identifies display adapters. Report
// their vendor/device IDs rather than guessing a marketing model name.
func pciGPUs(root string) string {
	devices, _ := filepath.Glob(filepath.Join(root, "*"))
	var names []string
	for _, path := range devices {
		class, err := strconv.ParseUint(strings.TrimSpace(readSystemFile(filepath.Join(path, "class"))), 0, 32)
		if err != nil || class>>16 != 3 {
			continue
		}
		vendor := strings.TrimPrefix(strings.TrimSpace(readSystemFile(filepath.Join(path, "vendor"))), "0x")
		device := strings.TrimPrefix(strings.TrimSpace(readSystemFile(filepath.Join(path, "device"))), "0x")
		if len(vendor) != 4 || len(device) != 4 {
			continue
		}
		if _, err := strconv.ParseUint(vendor+device, 16, 32); err != nil {
			continue
		}
		name := map[string]string{"8086": "Intel", "1002": "AMD", "10de": "NVIDIA"}[vendor]
		if name != "" {
			name += " "
		}
		names = append(names, name+"GPU ("+vendor+":"+device+")")
	}
	return gpuNames(names)
}

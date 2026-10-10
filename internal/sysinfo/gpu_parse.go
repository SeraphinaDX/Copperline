package sysinfo

import (
	"encoding/json"
	"regexp"
	"strings"
)

// Adapter APIs can repeat a name for multiple outputs. Show distinct model
// names and bound them before assembling the IRC message.
func gpuNames(names []string) string {
	seen := make(map[string]bool)
	var models []string
	for _, name := range names {
		name = clean(name, 64)
		id := strings.ToLower(name)
		if name != "" && !seen[id] {
			seen[id] = true
			models = append(models, name)
		}
	}
	return clean(strings.Join(models, "; "), 112)
}

var pciID = regexp.MustCompile(`\s+\[[0-9a-fA-F]{4}\]$`)

// lspci's verbose machine-readable format uses tab-separated tagged records.
// Numeric class IDs avoid locale-dependent or unknown class descriptions.
func lspciGPUs(data string) string {
	var names []string
	fields := make(map[string]string)
	flush := func() {
		classID := strings.TrimSpace(pciID.FindString(fields["Class"]))
		if strings.HasPrefix(classID, "[03") {
			vendor := pciID.ReplaceAllString(fields["Vendor"], "")
			device := pciID.ReplaceAllString(fields["Device"], "")
			if device != "" {
				names = append(names, strings.TrimSpace(vendor+" "+device))
			}
		}
		clear(fields)
	}
	for _, line := range strings.Split(data, "\n") {
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		key, value, ok := strings.Cut(line, ":\t")
		if ok {
			fields[key] = strings.TrimSpace(value)
		}
	}
	flush()
	return gpuNames(names)
}

func profilerGPUs(data string) string {
	var report struct {
		Displays []struct {
			Model string `json:"sppci_model"`
		} `json:"SPDisplaysDataType"`
	}
	if json.Unmarshal([]byte(data), &report) != nil {
		return ""
	}
	var names []string
	for _, display := range report.Displays {
		names = append(names, display.Model)
	}
	return gpuNames(names)
}

// Adreno and Mali expose model descriptions through these driver-owned files
// on some Android devices. Missing/SELinux-restricted files are normal.
func androidGPUs(read func(string) string) string {
	for _, path := range []string{
		"/sys/class/kgsl/kgsl-3d0/gpu_model",
		"/sys/class/misc/mali0/device/gpuinfo",
	} {
		if model := clean(read(path), 64); model != "" {
			return model
		}
	}
	return ""
}

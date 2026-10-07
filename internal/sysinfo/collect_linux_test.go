//go:build linux || android

package sysinfo

import "testing"

func TestCPUModelFromX86AndARM(t *testing.T) {
	for _, tc := range []struct{ data, want string }{
		{"processor : 0\nmodel name : Intel Example\nprocessor : 1\n", "Intel Example"},
		{"Processor : ARMv8\nHardware : Example mobile chip\n", "Example mobile chip"},
		{"Processor : ARMv8\n", "ARMv8"}, {"processor : 0\n", ""}, {"", ""},
	} {
		if got := cpuModel(tc.data); got != tc.want {
			t.Fatalf("cpu model = %q, want %q", got, tc.want)
		}
	}
}

func TestMemoryUsesAvailableRatherThanFree(t *testing.T) {
	total, available, known := memoryInfo("MemTotal: 8192 kB\nMemFree: 1024 kB\nMemAvailable: 4096 kB\n")
	if total != 8192*1024 || available != 4096*1024 || !known {
		t.Fatalf("memory = %d/%d known=%v", available, total, known)
	}
	for _, data := range []string{"", "MemTotal: invalid kB", "MemTotal: 8192 kB", "MemTotal: 8 kB\nMemAvailable: 16 kB", "MemTotal: 18446744073709551615 kB"} {
		if _, _, known := memoryInfo(data); known {
			t.Fatalf("invalid memory sample accepted: %q", data)
		}
	}
	if _, available, known := memoryInfo("MemTotal: 8 kB\nMemAvailable: 0 kB"); available != 0 || !known {
		t.Fatal("zero available RAM is a valid sample")
	}
}

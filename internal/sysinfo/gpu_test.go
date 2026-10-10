package sysinfo

import (
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

func TestLspciGPUModelsAndMultipleAdapters(t *testing.T) {
	data := "Slot:\t00:02.0\nClass:\tVGA compatible controller [0300]\nVendor:\tIntel Corporation [8086]\nDevice:\tUHD Graphics 620 [5917]\n\n" +
		"Slot:\t01:00.0\nClass:\t3D controller [0302]\nVendor:\tNVIDIA Corporation [10de]\nDevice:\tGeForce RTX 4060 [2882]\nRev:\ta1\n\n" +
		"Class:\tEthernet controller [0200]\nVendor:\tIntel Corporation [8086]\nDevice:\tEthernet [1111]\n\n" +
		"Class:\t3D controller [0302]\nVendor:\tNVIDIA Corporation [10de]\nDevice:\tGeForce RTX 4060 [2882]"
	want := "Intel Corporation UHD Graphics 620; NVIDIA Corporation GeForce RTX 4060"
	if got := lspciGPUs(data); got != want {
		t.Fatalf("GPU models = %q, want %q", got, want)
	}
	// Retain marketing names in brackets; strip only the trailing numeric ID.
	data = "Class:\tDisplay controller [0380]\nVendor:\tAMD [1002]\nDevice:\tNavi 31 [Radeon RX 7900 XTX] [744c]\n"
	if got := lspciGPUs(data); got != "AMD Navi 31 [Radeon RX 7900 XTX]" {
		t.Fatalf("GPU marketing name lost: %q", got)
	}
	for _, bad := range []string{"", "Class:\tunknown [zzzz]\nDevice:\tUnknown", "Class:\tAudio [0403]\nDevice:\tSound"} {
		if got := lspciGPUs(bad); got != "" {
			t.Fatalf("non-GPU record included: %q", got)
		}
	}
}

func TestProfilerReportsOnlyGPUModelNames(t *testing.T) {
	data := `{"SPDisplaysDataType":[{"sppci_model":"Apple M3","spdisplays_ndrvs":[{"_name":"Private monitor","serial":"private"}]},{"sppci_model":"AMD Radeon Pro 5500M"}]}`
	if got := profilerGPUs(data); got != "Apple M3; AMD Radeon Pro 5500M" {
		t.Fatalf("profiler GPU models = %q", got)
	}
	for _, data := range []string{"", "invalid", `{}`, `{"SPDisplaysDataType":[{}]}`} {
		if got := profilerGPUs(data); got != "" {
			t.Fatalf("unknown profiler output produced GPU: %q", got)
		}
	}
}

func TestAndroidGPUFilesAreBestEffort(t *testing.T) {
	for _, tc := range []struct{ path, text, want string }{
		{"/sys/class/kgsl/kgsl-3d0/gpu_model", "Adreno (TM) 740\n", "Adreno (TM) 740"},
		{"/sys/class/misc/mali0/device/gpuinfo", "Mali-G78 MP14 r0p0\n", "Mali-G78 MP14 r0p0"},
		{"", "", ""},
	} {
		read := func(path string) string {
			if path == tc.path {
				return tc.text
			}
			return "" // Unavailable or restricted files.
		}
		if got := androidGPUs(read); got != tc.want {
			t.Fatalf("Android GPU = %q, want %q", got, tc.want)
		}
	}
}

func TestGPUNamesSanitizeAndBoundMultipleAdapters(t *testing.T) {
	got := gpuNames([]string{"NVIDIA RTX", "nvidia rtx", "", "  AMD\r\nRadeon\x01  "})
	if got != "NVIDIA RTX; AMD Radeon" {
		t.Fatalf("GPU names = %q", got)
	}
	got = gpuNames([]string{strings.Repeat("界", 100), strings.Repeat("A", 100), strings.Repeat("B", 100)})
	if len(got) > 112 || !utf8.ValidString(got) || strings.ContainsFunc(got, unicode.IsControl) {
		t.Fatalf("invalid GPU summary: %q", got)
	}
}

func TestHardwareOutputIsBounded(t *testing.T) {
	var out hardwareOutput
	if _, err := out.Write(make([]byte, 64*1024)); err != nil {
		t.Fatal(err)
	}
	if _, err := out.Write([]byte("overflow")); err == nil || out.buffer.Len() != 64*1024 {
		t.Fatal("hardware tool output exceeded its limit")
	}
}

package sysinfo

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestHardwareCommandChild(t *testing.T) {
	// Run the test executable as a controlled hardware tool without depending
	// on a shell or on any utilities installed on the test machine.
	switch os.Args[len(os.Args)-1] {
	case "hardware-output-ok":
		_, _ = os.Stdout.WriteString("Example GPU\n")
		os.Exit(0)
	case "hardware-output-large":
		_, _ = os.Stdout.WriteString(strings.Repeat("X", 64*1024+1))
		os.Exit(0)
	case "hardware-output-block":
		time.Sleep(10 * time.Second)
		os.Exit(0)
	}
}

func TestHardwareCommandsHaveOutputAndTimeLimits(t *testing.T) {
	args := []string{"-test.run=^TestHardwareCommandChild$", "--", "hardware-output-ok"}
	if got := commandOutput(3*time.Second, os.Args[0], args...); got != "Example GPU\n" {
		t.Fatalf("hardware tool output = %q", got)
	}
	args[2] = "hardware-output-large"
	if got := commandOutput(3*time.Second, os.Args[0], args...); got != "" {
		t.Fatal("oversized hardware output was accepted")
	}
	args[2] = "hardware-output-block"
	start := time.Now()
	if got := commandOutput(100*time.Millisecond, os.Args[0], args...); got != "" {
		t.Fatal("timed-out hardware output was accepted")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("hardware query exceeded its bounded wait")
	}
}

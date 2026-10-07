//go:build (!linux && !android && !darwin && !windows) || ios

package sysinfo

// The portable baseline (OS, architecture and logical CPU count) remains
// available on platforms without an implementation of the optional statistics.
func collect(s *snapshot) {}

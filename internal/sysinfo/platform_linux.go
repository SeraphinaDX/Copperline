//go:build linux && !android

package sysinfo

import (
	"strconv"
	"strings"
)

func platform(s *snapshot) {
	s.os = "Linux"
	text := readSystemFile("/etc/os-release")
	if text == "" {
		text = readSystemFile("/usr/lib/os-release")
	}
	for _, line := range strings.Split(text, "\n") {
		key, value, ok := strings.Cut(line, "=")
		if ok && key == "PRETTY_NAME" {
			value = strings.TrimSpace(value)
			if unquoted, err := strconv.Unquote(value); err == nil {
				value = unquoted
			}
			if value != "" {
				s.os = value
			}
			return
		}
	}
}

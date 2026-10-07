package sysinfo

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

func platform(s *snapshot) {
	s.os = "Android"
	// These two public properties provide the Android release and device model.
	// Use a shared deadline: a missing/restricted getprop cannot stall input.
	ctx, cancel := context.WithTimeout(context.Background(), 750*time.Millisecond)
	defer cancel()
	property := func(key string) string {
		b, err := exec.CommandContext(ctx, "/system/bin/getprop", key).Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(b))
	}
	if release := property("ro.build.version.release"); release != "" {
		s.os += " " + release
	}
	s.device = property("ro.product.model")
}

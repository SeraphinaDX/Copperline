package sysinfo

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"time"
)

// Optional hardware tools get a fixed deadline and bounded output. No shell
// or network lookups are used; failure simply leaves the GPU field unavailable.
func commandOutput(timeout time.Duration, path string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.WaitDelay = 100 * time.Millisecond
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	out := &hardwareOutput{}
	cmd.Stdout = out
	if cmd.Run() != nil {
		return ""
	}
	return out.buffer.String()
}

// Do not embed bytes.Buffer: its promoted ReadFrom method would let io.Copy
// bypass Write and therefore bypass the output limit.
type hardwareOutput struct{ buffer bytes.Buffer }

func (b *hardwareOutput) Write(p []byte) (int, error) {
	if b.buffer.Len()+len(p) > 64*1024 {
		return 0, errors.New("hardware output exceeds limit")
	}
	return b.buffer.Write(p)
}

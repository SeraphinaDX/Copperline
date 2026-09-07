package logging

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"copperline/internal/config"
	"copperline/internal/model"
)

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._#&+!-]+`)

type Logger struct {
	mu           sync.Mutex
	enabled      bool
	dir          string
	timestamp    string
	sessionStart map[string]int64
}

func New(enabled bool, dir, timestamp string) *Logger {
	return &Logger{
		enabled:      enabled,
		dir:          config.ExpandPath(dir),
		timestamp:    timestamp,
		sessionStart: make(map[string]int64),
	}
}

// BeginBuffer records the byte offset where this Copperline process first
// became aware of a buffer. It must be called before a current-session message
// is exposed to the TUI. Doing that ordering explicitly prevents a first-view
// redraw from racing the logger and mistaking freshly received traffic for
// muted persistent backlog, especially for channels joined outside the config.
func (l *Logger) BeginBuffer(server, target string) error {
	if !l.enabled {
		return nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	return l.beginBufferLocked(server, target)
}

func (l *Logger) beginBufferLocked(server, target string) error {
	path := filepath.Join(l.dir, safe(server), safe(target)+".log")
	if _, ok := l.sessionStart[path]; ok {
		return nil
	}

	stat, err := os.Stat(path)
	switch {
	case err == nil:
		l.sessionStart[path] = stat.Size()
	case os.IsNotExist(err):
		l.sessionStart[path] = 0
	default:
		return err
	}
	return nil
}

func (l *Logger) Write(m model.Message) error {
	if !l.enabled {
		return nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.beginBufferLocked(m.Server, m.Target); err != nil {
		return err
	}
	server := safe(m.Server)
	target := safe(m.Target)
	dir := filepath.Join(l.dir, server)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(dir, target+".log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintln(f, model.FormatMessage(m, l.timestamp))
	return err
}

// Tail returns the final n plaintext log lines for a buffer without loading
// the whole log into memory. It is used to give newly opened buffers a small
// amount of persistent context while keeping the live TUI backlog bounded.
func (l *Logger) Tail(server, target string, n int) ([]string, error) {
	if !l.enabled || n <= 0 {
		return nil, nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	path := filepath.Join(l.dir, safe(server), safe(target)+".log")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	return tailLines(f, n)
}

func tailLines(f *os.File, n int) ([]string, error) {
	if n <= 0 {
		return nil, nil
	}
	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}
	return tailLinesAt(f, n, stat.Size())
}

// tailLinesAt is the bounded form of tailLines. end is an exclusive byte
// offset, allowing the caller to read history that existed before this
// Copperline process started appending to the file.
func tailLinesAt(f *os.File, n int, end int64) ([]string, error) {
	if n <= 0 || end <= 0 {
		return nil, nil
	}
	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if end > stat.Size() {
		end = stat.Size()
	}
	if end <= 0 {
		return nil, nil
	}

	const chunkSize int64 = 4096
	pos := end
	buf := make([]byte, 0, chunkSize)
	newlines := 0

	for pos > 0 && newlines <= n {
		readSize := chunkSize
		if pos < readSize {
			readSize = pos
		}
		pos -= readSize
		chunk := make([]byte, readSize)
		if _, err := f.ReadAt(chunk, pos); err != nil && err != io.EOF {
			return nil, err
		}
		buf = append(chunk, buf...)
		newlines = bytes.Count(buf, []byte{'\n'})
	}

	buf = bytes.TrimRight(buf, "\r\n")
	if len(buf) == 0 {
		return nil, nil
	}
	parts := bytes.Split(buf, []byte{'\n'})
	if len(parts) > n {
		parts = parts[len(parts)-n:]
	}
	lines := make([]string, 0, len(parts))
	for _, part := range parts {
		lines = append(lines, string(bytes.TrimSuffix(part, []byte{'\r'})))
	}
	return lines, nil
}

// BacklogTail returns the last n meaningful pre-session log lines for display
// when a conversational buffer is opened. Older Copperline versions logged routine channel-sync
// numerics, so filter those legacy entries here rather than resurrecting the
// protocol noise through the persistent backlog feature.
func (l *Logger) BacklogTail(server, target string, n int) ([]string, error) {
	if !l.enabled || n <= 0 {
		return nil, nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	path := filepath.Join(l.dir, safe(server), safe(target)+".log")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}
	end := stat.Size()
	if sessionStart, ok := l.sessionStart[path]; ok && sessionStart < end {
		// The muted preview is persistent context from before this process
		// started receiving messages. Current-session lines are rendered from
		// model.State instead, so opening a channel or PM query hours later cannot
		// recolor live traffic as log history.
		end = sessionStart
	}

	// Probe progressively farther back until we have n useful lines or have
	// reached the start of the pre-session portion of the file. This keeps
	// ordinary reads tiny while still coping with old logs containing a large
	// block of NAMES/WHO numerics.
	probe := n * 4
	if probe < 32 {
		probe = 32
	}
	for {
		lines, err := tailLinesAt(f, probe, end)
		if err != nil {
			return nil, err
		}
		filtered := make([]string, 0, len(lines))
		for _, line := range lines {
			if !isLegacyHousekeepingLogLine(line) {
				filtered = append(filtered, line)
			}
		}
		if len(filtered) >= n {
			return filtered[len(filtered)-n:], nil
		}
		if len(lines) < probe {
			return filtered, nil
		}
		probe *= 2
	}
}

func isLegacyHousekeepingLogLine(line string) bool {
	fields := strings.Fields(line)
	for i := 0; i+1 < len(fields); i++ {
		if fields[i] != "***" {
			continue
		}
		switch fields[i+1] {
		case "315", "324", "328", "329", "332", "333", "352", "353", "354", "366":
			return true
		}
	}
	return false
}

func safe(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "server"
	}
	return unsafeName.ReplaceAllString(s, "_")
}

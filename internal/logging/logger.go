package logging

import (
	"fmt"
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
	mu        sync.Mutex
	dir       string
	timestamp string
}

func New(dir, timestamp string) *Logger {
	return &Logger{dir: config.ExpandPath(dir), timestamp: timestamp}
}

func (l *Logger) Write(m model.Message) error {
	l.mu.Lock()
	defer l.mu.Unlock()
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

func safe(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "server"
	}
	return unsafeName.ReplaceAllString(s, "_")
}

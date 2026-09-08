package relay

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"copperline/internal/config"
	"copperline/internal/model"
)

type historySnapshot struct {
	Version  int
	Messages []model.Message
}

func newMessageID() string {
	var id [16]byte
	_, _ = rand.Read(id[:]) // crypto/rand.Read cannot fail on supported Go versions.
	return hex.EncodeToString(id[:])
}

func (s *Server) restoreHistory() error {
	if s.cfg.Relay.HistoryFile == "" {
		return nil
	}
	f, err := os.Open(config.ExpandPath(s.cfg.Relay.HistoryFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	var data historySnapshot
	dec := json.NewDecoder(f)
	if err := dec.Decode(&data); err != nil {
		return err
	}
	if data.Version != 1 {
		return fmt.Errorf("unsupported history format %d", data.Version)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return errors.New("trailing data in relay history")
	}
	// Validate completely before changing live state. A corrupt file is never
	// silently overwritten by an empty session.
	for _, msg := range data.Messages {
		if msg.RelayID == "" || msg.Server == "" || msg.Target == "" {
			return errors.New("invalid relay history message")
		}
	}
	for _, msg := range data.Messages {
		msg.Replay = false
		msg.SuppressNotify = false
		s.state.Add(msg) // enforces the current per-buffer history limit
	}
	return nil
}

func (s *Server) saveHistory() error {
	if s.cfg.Relay.HistoryFile == "" {
		return nil
	}
	path := config.ExpandPath(s.cfg.Relay.HistoryFile)
	buffers, _ := s.state.Snapshot()
	data := historySnapshot{Version: 1}
	for _, b := range buffers {
		data.Messages = append(data.Messages, b.Messages...)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".copperline-history-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err := json.NewEncoder(f).Encode(data); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

// Disk I/O runs independently of IRC delivery. Checkpoint once per second;
// clean shutdown joins this worker before writing the final snapshot.
func (s *Server) historyLoop(ctx context.Context) {
	if s.cfg.Relay.HistoryFile == "" {
		return
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !s.historyDirty.Swap(false) {
				continue
			}
			if err := s.saveHistory(); err != nil {
				s.historyDirty.Store(true)
				log.Printf("save relay history: %v", err)
			}
		}
	}
}

package gui

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"copperline/internal/config"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"
	"golang.org/x/crypto/ssh"
)

const starterConfig = `[general]
history_lines = 1000
sort_channels_alphabetically = true
show_join_messages = true
show_part_messages = true
show_quit_messages = true

[relay]
mode = "client"
address = "relay.example.com:2222"
user = "copperline"
host_key_fingerprint = ""
`

// Storage keeps mobile secrets in Fyne's app-private directory. ConfigPath is
// optional on desktop; the GUI never silently changes the TUI's configuration.
type Storage struct {
	App        fyne.App
	ConfigPath string
}

func (s Storage) LoadConfig() (string, error) {
	if s.ConfigPath != "" {
		data, err := os.ReadFile(s.ConfigPath)
		return string(data), err
	}
	r, err := s.App.Storage().Open("config.toml")
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, storage.ErrNotExists) {
		return starterConfig, nil
	}
	if err != nil {
		return "", err
	}
	defer r.Close()
	data, err := io.ReadAll(r)
	return string(data), err
}

func (s Storage) SaveConfig(data string) error {
	if s.ConfigPath != "" {
		path := s.ConfigPath
		f, err := os.CreateTemp(filepath.Dir(path), ".copperline-config-*")
		if err != nil {
			return err
		}
		defer os.Remove(f.Name())
		if _, err = f.WriteString(data); err != nil {
			f.Close()
			return err
		}
		if err = f.Close(); err != nil {
			return err
		}
		return os.Rename(f.Name(), path)
	}
	w, err := s.App.Storage().Save("config.toml")
	if errors.Is(err, storage.ErrNotExists) {
		w, err = s.App.Storage().Create("config.toml")
	}
	if err != nil {
		return err
	}
	if _, err = io.WriteString(w, data); err != nil {
		w.Close()
		return err
	}
	return w.Close()
}

func (s Storage) Signer() (ssh.Signer, error) {
	r, err := s.App.Storage().Open("relay_ed25519.pem")
	if err == nil {
		defer r.Close()
		data, err := io.ReadAll(r)
		if err != nil {
			return nil, err
		}
		return ssh.ParsePrivateKey(data)
	}
	// Never overwrite a key because of a permission or read failure.
	if !errors.Is(err, os.ErrNotExist) && !errors.Is(err, storage.ErrNotExists) {
		return nil, err
	}
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	w, err := s.App.Storage().Create("relay_ed25519.pem")
	if err != nil {
		return nil, err
	}
	if w.URI().Scheme() == "file" {
		if err = os.Chmod(w.URI().Path(), 0o600); err != nil {
			w.Close()
			return nil, err
		}
	}
	if _, err = w.Write(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})); err != nil {
		w.Close()
		return nil, err
	}
	if err = w.Close(); err != nil {
		return nil, err
	}
	return ssh.NewSignerFromKey(key)
}

func validateGUIConfig(data string) error {
	cfg, err := config.Decode(data)
	if err != nil {
		return err
	}
	if cfg.Relay.ModeValue() != "client" {
		return fmt.Errorf("Copperline GUI requires [relay].mode = \"client\"")
	}
	if cfg.Relay.InsecureSkipHostKey {
		return fmt.Errorf("set host_key_fingerprint; the GUI requires verified relay host keys")
	}
	return nil
}

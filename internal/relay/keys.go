package relay

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"copperline/internal/config"

	"golang.org/x/crypto/ssh"
)

func loadOrCreateHostSigner(path string) (ssh.Signer, error) {
	path = config.ExpandPath(path)
	data, err := os.ReadFile(path)
	if err == nil {
		return ssh.ParsePrivateKey(data)
	}
	if !os.IsNotExist(err) {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	der, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return nil, err
	}
	block := &pem.Block{Type: "PRIVATE KEY", Bytes: der}
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		return nil, err
	}
	return ssh.NewSignerFromKey(privateKey)
}

func ensureAuthorizedKeysFile(path string) error {
	path = config.ExpandPath(path)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	return f.Close()
}

func loadAuthorizedKeys(path string) (map[string]struct{}, error) {
	path = config.ExpandPath(path)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	keys := make(map[string]struct{})
	for len(bytes.TrimSpace(data)) > 0 {
		key, _, _, rest, err := ssh.ParseAuthorizedKey(data)
		if err != nil {
			// Permit blank/comment-only lines by advancing one line. ParseAuthorizedKey
			// already skips ordinary comments in most cases; this keeps malformed
			// input from producing an infinite loop and gives a useful error.
			line := data
			if i := bytes.IndexByte(data, '\n'); i >= 0 {
				line, rest = data[:i], data[i+1:]
			} else {
				rest = nil
			}
			if strings.TrimSpace(string(line)) != "" && !strings.HasPrefix(strings.TrimSpace(string(line)), "#") {
				return nil, fmt.Errorf("parse authorized key: %w", err)
			}
			data = rest
			continue
		}
		keys[string(key.Marshal())] = struct{}{}
		data = rest
	}
	return keys, nil
}

func loadOrCreateClientSigner(path, passphraseEnv string) (ssh.Signer, bool, string, error) {
	path = config.ExpandPath(path)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, false, "", err
		}
		_, privateKey, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, false, "", err
		}
		der, err := x509.MarshalPKCS8PrivateKey(privateKey)
		if err != nil {
			return nil, false, "", err
		}
		if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0o600); err != nil {
			return nil, false, "", err
		}
		signer, err := ssh.NewSignerFromKey(privateKey)
		if err != nil {
			return nil, false, "", err
		}
		pubPath := path + ".pub"
		if err := os.WriteFile(pubPath, ssh.MarshalAuthorizedKey(signer.PublicKey()), 0o644); err != nil {
			return nil, false, "", err
		}
		return signer, true, pubPath, nil
	}
	if err != nil {
		return nil, false, "", err
	}

	signer, err := ssh.ParsePrivateKey(data)
	if err != nil {
		var missing *ssh.PassphraseMissingError
		if !errors.As(err, &missing) {
			return nil, false, "", err
		}
		passphrase := ""
		if passphraseEnv != "" {
			passphrase = os.Getenv(passphraseEnv)
		}
		if passphrase == "" {
			return nil, false, "", fmt.Errorf("private key %s is encrypted; set [relay].private_key_passphrase_env", path)
		}
		signer, err = ssh.ParsePrivateKeyWithPassphrase(data, []byte(passphrase))
		if err != nil {
			return nil, false, "", err
		}
	}

	pubPath := path + ".pub"
	if _, err := os.Stat(pubPath); os.IsNotExist(err) {
		_ = os.WriteFile(pubPath, ssh.MarshalAuthorizedKey(signer.PublicKey()), 0o644)
	}
	return signer, false, pubPath, nil
}

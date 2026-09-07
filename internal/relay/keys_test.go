package relay

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestClientKeyGenerationWritesAuthorizedKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "relay_client_ed25519")
	signer, generated, pubPath, err := loadOrCreateClientSigner(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if !generated {
		t.Fatal("first key load did not report generated=true")
	}
	if signer == nil {
		t.Fatal("generated signer is nil")
	}
	pub, err := os.ReadFile(pubPath)
	if err != nil {
		t.Fatal(err)
	}
	parsed, _, _, _, err := ssh.ParseAuthorizedKey(pub)
	if err != nil {
		t.Fatalf("generated .pub is not authorized_keys format: %v", err)
	}
	if ssh.FingerprintSHA256(parsed) != ssh.FingerprintSHA256(signer.PublicKey()) {
		t.Fatal("generated public key does not match private key")
	}

	_, generatedAgain, _, err := loadOrCreateClientSigner(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if generatedAgain {
		t.Fatal("existing key was unexpectedly regenerated")
	}
}

func TestHostKeyGenerationIsStable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "relay_host_ed25519")
	first, err := loadOrCreateHostSigner(path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := loadOrCreateHostSigner(path)
	if err != nil {
		t.Fatal(err)
	}
	if ssh.FingerprintSHA256(first.PublicKey()) != ssh.FingerprintSHA256(second.PublicKey()) {
		t.Fatal("host key changed after reload")
	}
}

func TestAuthorizedKeysLoadsCopperlinePublicKey(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "client")
	signer, _, pubPath, err := loadOrCreateClientSigner(keyPath, "")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(pubPath)
	if err != nil {
		t.Fatal(err)
	}
	authorizedPath := filepath.Join(dir, "authorized_keys")
	if err := os.WriteFile(authorizedPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	keys, err := loadAuthorizedKeys(authorizedPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := keys[string(signer.PublicKey().Marshal())]; !ok {
		t.Fatal("generated client key was not accepted")
	}
}

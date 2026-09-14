package irc

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"copperline/internal/config"
	"copperline/internal/model"

	"github.com/lrstanley/girc"
)

func TestChannelHousekeepingNumericsAreHidden(t *testing.T) {
	hidden := []string{"315", "324", "328", "329", "332", "333", "352", "354", "353", "366"}
	for _, command := range hidden {
		if !isChannelHousekeepingNumeric(command) {
			t.Errorf("numeric %s should be treated as channel housekeeping", command)
		}
	}

	visible := []string{"001", "301", "311", "367", "368", "473", "475", "477"}
	for _, command := range visible {
		if isChannelHousekeepingNumeric(command) {
			t.Errorf("numeric %s should remain visible", command)
		}
	}
}

func writeTestClientCertificate(t *testing.T) (string, string) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "copperline-test"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	certificateDER, err := x509.CreateCertificate(rand.Reader, template, template, publicKey, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certPath := filepath.Join(dir, "client-cert.pem")
	keyPath := filepath.Join(dir, "client-key.pem")
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificateDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	return certPath, keyPath
}

func TestNewLoadsTLSClientCertificateForSASLExternal(t *testing.T) {
	certPath, keyPath := writeTestClientCertificate(t)
	cfg := &config.Config{Servers: []config.ServerConfig{{
		Name: "libera", Host: "irc.libera.chat", Port: 6697, TLS: true,
		Nick: "tester", User: "tester", TLSCertFile: certPath, TLSKeyFile: keyPath,
		SASL: config.SASL{Mechanism: "external"},
	}}}
	m := New(cfg, nil)
	s := m.sessions["libera"]
	if s.setupErr != nil {
		t.Fatalf("loading client certificate failed: %v", s.setupErr)
	}
	if s.client.Config.TLSConfig == nil || len(s.client.Config.TLSConfig.Certificates) != 1 {
		t.Fatal("TLS client certificate was not attached to the IRC connection")
	}
	if _, ok := s.client.Config.SASL.(*girc.SASLExternal); !ok {
		t.Fatalf("SASL mechanism = %T, want *girc.SASLExternal", s.client.Config.SASL)
	}
}

func TestNewLoadsCombinedCertificateAndKeyPEM(t *testing.T) {
	certPath, keyPath := writeTestClientCertificate(t)
	certificatePEM, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	combinedPath := filepath.Join(t.TempDir(), "libera.pem")
	if err := os.WriteFile(combinedPath, append(certificatePEM, keyPEM...), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Servers: []config.ServerConfig{{
		Name: "libera", Host: "irc.libera.chat", Port: 6697, TLS: true,
		Nick: "tester", User: "tester", TLSCertFile: combinedPath, TLSKeyFile: combinedPath,
	}}}
	m := New(cfg, nil)
	if s := m.sessions["libera"]; s.setupErr != nil || len(s.client.Config.TLSConfig.Certificates) != 1 {
		t.Fatalf("combined PEM was not loaded: setupErr=%v", s.setupErr)
	}
}

func TestUnreadableTLSClientCertificateBlocksConnection(t *testing.T) {
	cfg := &config.Config{Servers: []config.ServerConfig{{
		Name: "libera", Host: "irc.libera.chat", Port: 6697, TLS: true,
		Nick: "tester", User: "tester", TLSCertFile: "missing-cert.pem", TLSKeyFile: "missing-key.pem",
		SASL: config.SASL{Mechanism: "external"},
	}}}
	m := New(cfg, nil)
	if err := m.ConnectServer("libera"); err == nil {
		t.Fatal("ConnectServer accepted an unreadable TLS client certificate")
	}
	if m.WantsConnection("libera") {
		t.Fatal("unreadable TLS client certificate left auto-reconnect enabled")
	}
}

func TestOnlyJoinPartAndQuitSuppressUnreadActivity(t *testing.T) {
	var messages []model.Message
	m := &Manager{
		cfg:          &config.Config{},
		sessions:     make(map[string]*Session),
		typing:       make(map[string]typingEntry),
		knownTargets: make(map[string]map[string]struct{}),
		emit:         func(msg model.Message) { messages = append(messages, msg) },
	}
	c := girc.New(girc.Config{Server: "irc.test", Nick: "tester", User: "tester"})
	source := &girc.Source{Name: "alice"}

	cases := []struct {
		command  string
		params   []string
		suppress bool
	}{
		{girc.JOIN, []string{"#chan"}, true},
		{girc.PART, []string{"#chan", "bye"}, true},
		{girc.QUIT, []string{"gone"}, true},
		{girc.KICK, []string{"#chan", "bob", "reason"}, false},
		{girc.TOPIC, []string{"#chan", "new topic"}, false},
	}
	for _, tc := range cases {
		messages = nil
		m.handleEvent("test", c, girc.Event{Command: tc.command, Params: tc.params, Source: source})
		if len(messages) != 1 {
			t.Fatalf("%s emitted %d messages, want 1: %#v", tc.command, len(messages), messages)
		}
		if messages[0].SuppressUnread != tc.suppress {
			t.Fatalf("%s SuppressUnread = %t, want %t", tc.command, messages[0].SuppressUnread, tc.suppress)
		}
	}
}

package gotify

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestWorkerContinuesAfterDeliveryFailure(t *testing.T) {
	var attempts atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	n := &Notifier{enabled: true, endpoint: server.URL, client: &http.Client{Timeout: time.Second}, queue: make(chan message, 2)}
	n.queue <- message{Message: "first"}
	n.queue <- message{Message: "second"}
	close(n.queue)
	n.worker()
	status := n.Status()
	if !strings.Contains(status, "sent=1 failed=1") || strings.Contains(status, "last success=never") || !strings.Contains(status, "last error=none") {
		t.Fatalf("status: %s", status)
	}
}

func TestDeliveryFailureAndQueueOverflowAreVisible(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte("private response content"))
	}))
	defer server.Close()
	n := &Notifier{enabled: true, endpoint: server.URL, client: &http.Client{Timeout: time.Second}, queue: make(chan message, 1)}
	n.Send("mention", "one")
	n.Send("mention", "two")
	close(n.queue)
	n.worker()
	status := n.Status()
	if !strings.Contains(status, "failed=1 dropped=1") || !strings.Contains(status, "HTTP 401 Unauthorized") {
		t.Fatalf("status: %s", status)
	}
	if strings.Contains(status, "private response content") || strings.Contains(status, server.URL) {
		t.Fatal("status exposed private request details")
	}
}

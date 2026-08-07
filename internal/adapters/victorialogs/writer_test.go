package victorialogs

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"event_exporter/internal/domain"
	"event_exporter/internal/pkg/logger"
)

func testEntry(t *testing.T, msg string) *domain.LogEntry {
	t.Helper()
	entry, err := domain.NewLogEntry(
		time.Date(2026, time.March, 29, 12, 0, 0, 0, time.UTC),
		"warning",
		"event",
		msg,
		map[string]string{"k8s.namespace": "monitoring"},
	)
	if err != nil {
		t.Fatalf("NewLogEntry returned error: %v", err)
	}
	return entry
}

// countingServer records POST bodies and lets the test script per-request
// status codes.
type countingServer struct {
	mu       sync.Mutex
	requests int
	lines    []string
	statuses []int // consumed in order; last value repeats
}

func (s *countingServer) handler(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	status := http.StatusOK
	if len(s.statuses) > 0 {
		status = s.statuses[0]
		if len(s.statuses) > 1 {
			s.statuses = s.statuses[1:]
		}
	}

	if status == http.StatusOK {
		scanner := bufio.NewScanner(r.Body)
		for scanner.Scan() {
			if line := strings.TrimSpace(scanner.Text()); line != "" {
				s.lines = append(s.lines, line)
			}
		}
	}

	s.requests++
	w.WriteHeader(status)
}

func (s *countingServer) requestCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.requests
}

func (s *countingServer) lineCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.lines)
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return cond()
}

func newTestWriter(t *testing.T, endpoint string, mutate func(*VictoriaLogsConfig)) *Writer {
	t.Helper()

	cfg := VictoriaLogsConfig{
		Enabled:   true,
		Endpoint:  endpoint,
		ClusterID: "test",
		BatchSize: 1,
		FlushTime: time.Hour, // flush is driven by batch size unless a test overrides
		Timeout:   2 * time.Second,
	}
	if mutate != nil {
		mutate(&cfg)
	}

	w, err := newWriter(cfg, logger.New("error"))
	if err != nil {
		t.Fatalf("newWriter returned error: %v", err)
	}
	if w == nil {
		t.Fatal("newWriter returned nil writer for enabled config")
	}
	// Shorten retry backoff before the delivery loop starts.
	w.initialBackoff = time.Millisecond
	w.maxBackoff = 5 * time.Millisecond
	w.start()
	return w
}

func TestNewWriterRejectsInvalidEndpoint(t *testing.T) {
	t.Parallel()

	_, err := NewWriter(VictoriaLogsConfig{
		Enabled:  true,
		Endpoint: "http://bad endpoint with spaces",
	}, logger.New("error"))
	if err == nil {
		t.Fatal("expected error for a structurally invalid endpoint URL")
	}
}

func TestWriterRetriesUntilDelivered(t *testing.T) {
	t.Parallel()

	srv := &countingServer{statuses: []int{http.StatusInternalServerError, http.StatusBadGateway, http.StatusOK}}
	ts := httptest.NewServer(http.HandlerFunc(srv.handler))
	defer ts.Close()

	w := newTestWriter(t, ts.URL, nil)
	defer w.Stop()

	if err := w.Write(context.Background(), []*domain.LogEntry{testEntry(t, "retry me")}); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	if !waitFor(t, 2*time.Second, func() bool { return srv.lineCount() == 1 }) {
		t.Fatalf("expected 1 delivered line after retries, got %d (requests: %d)", srv.lineCount(), srv.requestCount())
	}
	if srv.requestCount() != 3 {
		t.Fatalf("expected 3 attempts (2 failures + 1 success), got %d", srv.requestCount())
	}
}

func TestWriterDropsBatchOnNonRetriableError(t *testing.T) {
	t.Parallel()

	srv := &countingServer{statuses: []int{http.StatusBadRequest}}
	ts := httptest.NewServer(http.HandlerFunc(srv.handler))
	defer ts.Close()

	w := newTestWriter(t, ts.URL, nil)
	defer w.Stop()

	if err := w.Write(context.Background(), []*domain.LogEntry{testEntry(t, "poison")}); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	if !waitFor(t, 2*time.Second, func() bool { return srv.requestCount() >= 1 }) {
		t.Fatal("expected at least one request")
	}
	// Give the writer a chance to (incorrectly) retry.
	time.Sleep(50 * time.Millisecond)
	if srv.requestCount() != 1 {
		t.Fatalf("expected exactly 1 attempt for a rejected batch, got %d", srv.requestCount())
	}
}

func TestWriterFlushesBufferedEventsOnStop(t *testing.T) {
	t.Parallel()

	srv := &countingServer{}
	ts := httptest.NewServer(http.HandlerFunc(srv.handler))
	defer ts.Close()

	w := newTestWriter(t, ts.URL, func(cfg *VictoriaLogsConfig) {
		cfg.BatchSize = 100 // keep events buffered until shutdown
	})

	entries := []*domain.LogEntry{
		testEntry(t, "one"),
		testEntry(t, "two"),
		testEntry(t, "three"),
	}
	if err := w.Write(context.Background(), entries); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	w.Stop() // blocks until the final flush completes

	if srv.lineCount() != 3 {
		t.Fatalf("expected 3 lines delivered by final flush, got %d", srv.lineCount())
	}
}

func TestWriterSendsAuthAndCustomHeaders(t *testing.T) {
	t.Parallel()

	var (
		mu      sync.Mutex
		gotAuth string
		gotOrg  string
	)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotAuth = r.Header.Get("Authorization")
		gotOrg = r.Header.Get("X-Scope-OrgID")
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	w := newTestWriter(t, ts.URL, func(cfg *VictoriaLogsConfig) {
		cfg.Auth.BearerToken = "secret-token"
		cfg.Headers = map[string]string{"X-Scope-OrgID": "tenant-1"}
	})
	defer w.Stop()

	if err := w.Write(context.Background(), []*domain.LogEntry{testEntry(t, "with auth")}); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	if !waitFor(t, 2*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return gotAuth != ""
	}) {
		t.Fatal("server did not receive a request")
	}

	mu.Lock()
	defer mu.Unlock()
	if gotAuth != "Bearer secret-token" {
		t.Fatalf("unexpected Authorization header: %q", gotAuth)
	}
	if gotOrg != "tenant-1" {
		t.Fatalf("unexpected X-Scope-OrgID header: %q", gotOrg)
	}
}

func TestBuildHeadersRejectsConflictingAuth(t *testing.T) {
	t.Parallel()

	_, err := buildHeaders(nil, AuthConfig{BasicUsername: "u", BasicPassword: "p", BearerToken: "t"})
	if err == nil {
		t.Fatal("expected error when both basic auth and bearer token are configured")
	}

	_, err = buildHeaders(nil, AuthConfig{BasicPassword: "p"})
	if err == nil {
		t.Fatal("expected error when basic password is set without username")
	}
}

func TestBuildHeadersBasicAuth(t *testing.T) {
	t.Parallel()

	headers, err := buildHeaders(nil, AuthConfig{BasicUsername: "user", BasicPassword: "pass"})
	if err != nil {
		t.Fatalf("buildHeaders returned error: %v", err)
	}
	// "user:pass" in base64
	if headers["Authorization"] != "Basic dXNlcjpwYXNz" {
		t.Fatalf("unexpected Authorization header: %q", headers["Authorization"])
	}
}

func TestBuildTransportRequiresCertAndKeyTogether(t *testing.T) {
	t.Parallel()

	_, err := buildTransport(TLSConfig{CertFile: "/tmp/cert.pem"})
	if err == nil {
		t.Fatal("expected error when cert_file is set without key_file")
	}
}

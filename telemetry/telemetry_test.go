package telemetry

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSetupDisabledIsNoop(t *testing.T) {
	Disable()
	t.Cleanup(Disable)
	if err := Setup(context.Background(), Config{Enabled: false, Endpoint: "http://127.0.0.1:4318"}); err != nil {
		t.Fatalf("disabled Setup should not error: %v", err)
	}
	if Enabled() {
		t.Fatal("telemetry should be disabled")
	}
	_, span := Start(context.Background(), "probe")
	defer span.End()
	if span.IsRecording() {
		t.Fatal("no-op provider must not record spans")
	}
}

func TestSetupEnabledWithoutEndpointStaysOff(t *testing.T) {
	Disable()
	t.Cleanup(Disable)
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
	if err := Setup(context.Background(), Config{Enabled: true, Endpoint: ""}); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if Enabled() {
		t.Fatal("no endpoint configured; exporter must stay off")
	}
}

func TestSetupEnabledExportsToOTLP(t *testing.T) {
	Disable()
	t.Cleanup(Disable)
	t.Setenv("OTEL_BSP_SCHEDULE_DELAY", "10")

	var hits atomic.Int32
	var body atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if r.URL.Path == "/v1/traces" {
			body.Store(string(raw))
			hits.Add(1)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := Setup(ctx, Config{
		Enabled: true, Endpoint: srv.URL, SampleRatio: 1,
		ServiceName: "kaiyang-test", ServiceVersion: "test",
	}); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if !Enabled() {
		t.Fatal("telemetry should be enabled")
	}
	_, span := Start(ctx, "probe")
	span.End()
	// Shutdown flushes the batch; the test must not depend on the batching timer.
	Disable()

	deadline := time.Now().Add(5 * time.Second)
	for hits.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if hits.Load() == 0 {
		t.Fatal("exporter did not POST to /v1/traces")
	}
	if b, _ := body.Load().(string); !strings.Contains(b, "probe") {
		t.Fatalf("exported payload does not mention span name: %q", b)
	}
}

func TestMiddlewarePassThroughWhenDisabled(t *testing.T) {
	Disable()
	t.Cleanup(Disable)
	called := false
	h := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusTeapot)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/tasks/1?token=secret", nil))
	if !called || rec.Code != http.StatusTeapot {
		t.Fatalf("handler not passed through: called=%v code=%d", called, rec.Code)
	}
}

func TestRouteLabelSanitizesIDs(t *testing.T) {
	cases := map[string]string{
		"/api/tasks/123":            "/api/tasks/{id}",
		"/api/tasks/123/findings/9": "/api/tasks/{id}/findings/{id}",
		"/api/none":                 "/api/none",
		"/api/tasks/550e8400-e29b-41d4-a716-446655440000": "/api/tasks/{id}",
	}
	for in, want := range cases {
		if got := RouteLabel(in); got != want {
			t.Fatalf("RouteLabel(%q)=%q, want %q", in, got, want)
		}
	}
}

func TestParseSampleRatio(t *testing.T) {
	cases := []struct {
		raw  string
		def  float64
		want float64
	}{
		{"", 1, 1},
		{"0.25", 1, 0.25},
		{"2", 1, 1},
		{"-1", 1, 0},
		{"nope", 0.5, 0.5},
	}
	for _, c := range cases {
		if got := ParseSampleRatio(c.raw, c.def); got != c.want {
			t.Fatalf("ParseSampleRatio(%q,%v)=%v, want %v", c.raw, c.def, got, c.want)
		}
	}
}

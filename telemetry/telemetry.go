// Package telemetry is the smallest useful OpenTelemetry wiring for KAIYANG.
//
// Default is a pure no-op: without an explicit OTLP endpoint the global tracer
// provider stays the OTel no-op provider, so existing deployments pay no network
// cost and every span call is a cheap no-op. Export is OTLP/HTTP; the endpoint
// comes from the otel_endpoint setting or the standard
// OTEL_EXPORTER_OTLP_TRACES_ENDPOINT / OTEL_EXPORTER_OTLP_ENDPOINT env var.
//
// No request/response bodies, prompts, credentials or tool arguments are
// recorded: spans only carry low-cardinality metadata (method, route, status,
// task/intent ids, model name).
package telemetry

import (
	"context"
	"fmt"
	"net/http"
	neturl "net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

const tracerName = "github.com/3169417664/kaiyang"

// Config is the resolved runtime configuration. Enabled alone is not enough:
// an endpoint must also be configured, otherwise Setup stays a no-op.
type Config struct {
	Enabled        bool
	Endpoint       string
	SampleRatio    float64
	ServiceName    string
	ServiceVersion string
}

// Status reports what Setup installed. It is diagnostics only.
type Status struct {
	Enabled  bool
	Endpoint string
	Sample   float64
}

var (
	mu      sync.Mutex
	current *sdktrace.TracerProvider
	status  atomic.Pointer[Status]
)

func init() {
	status.Store(&Status{})
}

// Enabled reports whether an OTLP exporter is currently installed.
func Enabled() bool {
	st := status.Load()
	return st != nil && st.Enabled
}

// Endpoint returns the exporter endpoint currently in use ("" when disabled).
func Endpoint() string {
	st := status.Load()
	if st == nil {
		return ""
	}
	return st.Endpoint
}

// Setup installs (or removes) the global tracer provider. Calling it again
// replaces the previous provider and shuts it down, so settings changes take
// effect without a restart. It never returns an error for "disabled"; errors are
// reserved for an explicitly enabled but unusable exporter.
func Setup(ctx context.Context, cfg Config) error {
	mu.Lock()
	defer mu.Unlock()

	if strings.EqualFold(strings.TrimSpace(os.Getenv("OTEL_SDK_DISABLED")), "true") {
		cfg.Enabled = false
	}
	endpoint := strings.TrimSpace(cfg.Endpoint)
	if !cfg.Enabled {
		endpoint = ""
	} else if endpoint == "" {
		// Explicit standard env vars are still honored; anything else means the
		// operator enabled tracing without telling us where to send it.
		endpoint = firstNonEmpty(
			os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"),
			os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		)
	}
	if endpoint == "" {
		return disableLocked()
	}

	u, err := neturl.Parse(endpoint)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("otel_endpoint 不是合法的 OTLP/HTTP 地址：%q", endpoint)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("otel_endpoint 只支持 http/https：%q", endpoint)
	}
	// WithEndpointURL overrides the whole path, so a bare host would POST to "/".
	// Fill in the OTLP/HTTP default unless the operator gave an explicit path.
	if u.Path == "" || u.Path == "/" {
		u.Path = "/v1/traces"
	}
	ratio := normalizeRatio(cfg.SampleRatio)
	opts := []otlptracehttp.Option{otlptracehttp.WithEndpointURL(u.String())}
	expCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	exp, err := otlptracehttp.New(expCtx, opts...)
	if err != nil {
		return fmt.Errorf("初始化 OTLP exporter 失败：%w", err)
	}
	name := strings.TrimSpace(cfg.ServiceName)
	if name == "" {
		name = "kaiyang"
	}
	version := strings.TrimSpace(cfg.ServiceVersion)
	attrs := []attribute.KeyValue{attribute.String("service.name", name)}
	if version != "" {
		attrs = append(attrs, attribute.String("service.version", version))
	}
	res, err := resource.New(ctx, resource.WithAttributes(attrs...))
	if err != nil {
		res = resource.Default()
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))),
	)
	if current != nil {
		shutdownProvider(current)
	}
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))
	current = tp
	status.Store(&Status{Enabled: true, Endpoint: endpoint, Sample: ratio})
	return nil
}

// Disable removes any installed provider and returns the global tracer to the
// no-op implementation. Safe to call when tracing was never enabled.
func Disable() {
	mu.Lock()
	defer mu.Unlock()
	_ = disableLocked()
}

func disableLocked() error {
	if current != nil {
		shutdownProvider(current)
		current = nil
	}
	otel.SetTracerProvider(noop.NewTracerProvider())
	status.Store(&Status{})
	return nil
}

// shutdownProvider flushes buffered spans with a bounded wait. A background
// context with no deadline can hang a settings change or process shutdown when
// the collector is unreachable; losing the tail of the buffer is preferable.
func shutdownProvider(tp *sdktrace.TracerProvider) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = tp.Shutdown(ctx)
}

// Start begins a span. With the default no-op provider this is cheap and safe.
func Start(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return otel.Tracer(tracerName).Start(ctx, name, trace.WithAttributes(attrs...))
}

// ParseSampleRatio converts a settings/env value into a ratio. Invalid or empty
// input falls back to def; the result is clamped to [0,1].
func ParseSampleRatio(raw string, def float64) float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return normalizeRatio(def)
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return normalizeRatio(def)
	}
	return normalizeRatio(v)
}

func normalizeRatio(v float64) float64 {
	if v != v { // NaN
		return 1
	}
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

// statusWriter records the response status for the span without buffering the
// body. Flush is forwarded: SSE endpoints (/api/logs/stream, /api/update/stream)
// type-assert http.Flusher and must keep working.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// Middleware traces one HTTP request. It is a pass-through when tracing is off,
// so the disabled path adds only one atomic load. Query strings are deliberately
// not recorded (they can carry tokens); the path is normalized to keep span
// cardinality bounded.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !Enabled() {
			next.ServeHTTP(w, r)
			return
		}
		ctx, span := Start(r.Context(), "http.request",
			attribute.String("http.method", r.Method),
			attribute.String("http.route", RouteLabel(r.URL.Path)),
		)
		defer span.End()
		sw := &statusWriter{ResponseWriter: w, status: 0}
		next.ServeHTTP(sw, r.WithContext(ctx))
		code := sw.status
		if code == 0 {
			code = http.StatusOK
		}
		span.SetAttributes(attribute.Int("http.status_code", code))
		if code >= 500 {
			span.SetStatus(codes.Error, http.StatusText(code))
		}
	})
}

// RouteLabel normalizes a request path for span attributes: numeric and
// uuid-like segments become {id}, the result is capped at 160 bytes. This keeps
// task ids, finding ids and exchange ids from exploding the metric cardinality.
func RouteLabel(path string) string {
	if path == "" {
		return "/"
	}
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if p == "" {
			continue
		}
		if isNumeric(p) || isUUIDLike(p) || len(p) > 64 {
			parts[i] = "{id}"
		}
	}
	out := strings.Join(parts, "/")
	if len(out) > 160 {
		out = out[:160]
	}
	return out
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func isUUIDLike(s string) bool {
	if len(s) < 16 || len(s) > 64 {
		return false
	}
	dashes, hex := 0, 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '-':
			dashes++
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
			hex++
		default:
			return false
		}
	}
	return hex >= 16 && dashes <= 4
}

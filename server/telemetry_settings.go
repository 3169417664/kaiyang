// telemetry_settings.go：OpenTelemetry 追踪（上游 v0.4.118 P2-3）的设置读写。
//
// 三个设置键（默认关闭）：
//   - otel_enabled       true/false
//   - otel_endpoint      OTLP/HTTP endpoint（如 http://127.0.0.1:4318）；为空时不导出
//   - otel_sample_ratio  0–1，默认 1
//
// 只有显式配置 endpoint 才会创建 OTLP exporter；其余情况全局 tracer 保持 no-op，
// 不联网、不额外分配。设置变更即时重建 provider，无需重启。
package server

import (
	"context"
	"log"
	"strings"

	"github.com/3169417664/kaiyang/db"
	"github.com/3169417664/kaiyang/telemetry"
)

const (
	settingOTELEnabled     = "otel_enabled"
	settingOTELEndpoint    = "otel_endpoint"
	settingOTELSampleRatio = "otel_sample_ratio"

	defaultOTELSampleRatio = 1.0
)

// otelConfigFromPG resolves the runtime config. enabled defaults to false and an
// empty endpoint keeps the exporter off even if enabled was set.
func otelConfigFromPG(pg *db.DB) telemetry.Config {
	cfg := telemetry.Config{
		ServiceName:    "kaiyang",
		ServiceVersion: BuildVersion,
		SampleRatio:    defaultOTELSampleRatio,
	}
	if pg == nil {
		return cfg
	}
	cfg.Enabled = pg.GetBool(settingOTELEnabled, false)
	endpoint, _, _ := pg.GetSetting(settingOTELEndpoint)
	cfg.Endpoint = strings.TrimSpace(endpoint)
	ratio, _, _ := pg.GetSetting(settingOTELSampleRatio)
	cfg.SampleRatio = telemetry.ParseSampleRatio(ratio, defaultOTELSampleRatio)
	return cfg
}

// applyTelemetry installs or removes the tracer provider from current settings.
// Errors are logged, never fatal: tracing is an observability side channel and
// must not take the server down.
func (s *Server) applyTelemetry(ctx context.Context) {
	if s == nil || s.m == nil || s.m.pg == nil {
		return
	}
	cfg := otelConfigFromPG(s.m.pg)
	if err := telemetry.Setup(ctx, cfg); err != nil {
		log.Printf("[telemetry] 初始化失败（保持关闭）: %v", err)
		return
	}
	if telemetry.Enabled() {
		log.Printf("[telemetry] OTLP/HTTP 已启用：endpoint=%s sample=%.3f", telemetry.Endpoint(), cfg.SampleRatio)
	} else {
		log.Printf("[telemetry] 未启用（默认 no-op，无网络开销）")
	}
}

// otelSettingsEcho is the /api/settings projection.
func otelSettingsEcho(pg *db.DB) (enabled bool, endpoint string, ratio float64) {
	cfg := otelConfigFromPG(pg)
	return cfg.Enabled, cfg.Endpoint, cfg.SampleRatio
}

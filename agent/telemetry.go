package agent

// 工具调用 span。agent 级 span 在 planner/worker/mainagent 入口创建；这里包住
// 每次 CoreTool.Call，记录工具名、错误与结果大小。默认 no-op provider 时不产生
// 任何网络/性能负担，也不记录入参或结果正文。

import (
	"context"
	"encoding/json"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/3169417664/kaiyang/telemetry"
	actool "github.com/Autumn-27/norma/tool"
)

type tracedTool struct{ actool.CoreTool }

func (t *tracedTool) AcceptsRawInput() bool {
	if r, ok := t.CoreTool.(interface{ AcceptsRawInput() bool }); ok {
		return r.AcceptsRawInput()
	}
	return false
}

func (t *tracedTool) Call(ctx context.Context, in json.RawMessage, tc *actool.ToolContext) (actool.Result, error) {
	ctx, span := telemetry.Start(ctx, "execute_tool."+t.Name(),
		attribute.String("tool.name", t.Name()),
	)
	defer span.End()
	res, err := t.CoreTool.Call(ctx, in, tc)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(attribute.Bool("tool.is_error", true))
		return res, err
	}
	span.SetAttributes(
		attribute.Bool("tool.is_error", res.IsError),
		attribute.Int("tool.result_chars", len([]rune(res.Flatten()))),
	)
	if res.IsError {
		span.SetStatus(codes.Error, "tool returned error result")
	}
	return res, nil
}

// traceTool wraps one tool for span emission. Wrapping is unconditional so a
// later enable (settings change) takes effect without rebuilding agents; with
// the no-op provider the call is a couple of cheap interface hops.
func traceTool(t actool.CoreTool) actool.CoreTool { return &tracedTool{CoreTool: t} }

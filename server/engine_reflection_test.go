package server

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/3169417664/kaiyang/agent"
)

func TestBuildReflectionLesson(t *testing.T) {
	wrote := agent.WriteCounts{Facts: 1, Assets: 2, Findings: 1}
	cases := []struct {
		state    string
		reason   string
		wrote    agent.WriteCounts
		calls    int
		errs     int
		contains []string
	}{
		{"done", "", wrote, 12, 1, []string{"完成且有写回", "事实1", "工具12次"}},
		{"done", "", agent.WriteCounts{}, 3, 0, []string{"完成但零写回", "工具3次"}},
		{"exhausted", "max_turns: hit limit", agent.WriteCounts{}, 20, 4, []string{"耗尽无产出", "max_turns", "不要原样重试"}},
		{"blocked", "error: boom", agent.WriteCounts{}, 5, 5, []string{"阻塞", "error", "先排除"}},
		{"stopped", "killed", agent.WriteCounts{}, 1, 0, []string{"被终止", "killed"}},
		{"weird", "", agent.WriteCounts{}, 0, 0, []string{"终态 weird"}},
	}
	for _, tc := range cases {
		got := buildReflectionLesson(tc.state, tc.reason, "原方向摘要", "拿到 admin cookie", tc.wrote, tc.calls, tc.errs)
		for _, want := range tc.contains {
			if !strings.Contains(got, want) {
				t.Errorf("state=%s lesson %q missing %q", tc.state, got, want)
			}
		}
	}
}

func TestBuildReflectionLessonIncludesSummaryAndCriteria(t *testing.T) {
	got := buildReflectionLesson("blocked", "waf: 403", "测试 /api/user?id=1 的越权",
		"拿到他人订单数据", agent.WriteCounts{}, 8, 2)
	for _, want := range []string{"原方向", "/api/user", "验收标准", "他人订单"} {
		if !strings.Contains(got, want) {
			t.Errorf("lesson missing %q: %s", want, got)
		}
	}
}

func TestSanitizeReflectionText(t *testing.T) {
	// Invalid UTF-8 must not reach PostgreSQL text columns.
	got := sanitizeReflectionText("ok\xffbad")
	if !strings.Contains(got, "ok") || !strings.Contains(got, "bad") {
		t.Fatalf("sanitize lost content: %q", got)
	}
	if !isValidUTF8(got) {
		t.Fatalf("sanitize left invalid UTF-8: %q", got)
	}
}

func TestTruncateUTF8BytesKeepsRuneBoundary(t *testing.T) {
	s := "中文摘要中文摘要"
	got := truncateUTF8Bytes(s, 7)
	if len(got) > 7 {
		t.Fatalf("too long: %d", len(got))
	}
	if !isValidUTF8(got) {
		t.Fatalf("split rune: %q", got)
	}
	if got != "中文" {
		t.Fatalf("truncate = %q, want 中文", got)
	}
}

func isValidUTF8(s string) bool { return utf8.ValidString(s) }

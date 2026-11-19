package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// 批量导入的行解析：用户从付费站导出的格式五花八门，解析器必须认得常见几种，
// 并且不把注释/空行当条目。
func TestParseProxyLine(t *testing.T) {
	cases := []struct {
		raw      string
		wantURL  string
		wantName string
		comment  bool
	}{
		{"1.2.3.4:8080", "http://1.2.3.4:8080", "", false},
		{"http://1.2.3.4:8080", "http://1.2.3.4:8080", "", false},
		{"socks5://user:pass@5.6.7.8:1080", "socks5://user:pass@5.6.7.8:1080", "", false},
		{"1.2.3.4:8080:user:pass", "http://user:pass@1.2.3.4:8080", "", false},
		{"HK-01 | 1.2.3.4:8080", "http://1.2.3.4:8080", "HK-01", false},
		{"# comment", "", "", true},
		{"", "", "", true},
	}
	for _, c := range cases {
		name, u, comment, err := parseProxyLine(c.raw)
		if err != nil {
			t.Errorf("parseProxyLine(%q) err=%v", c.raw, err)
			continue
		}
		if comment != c.comment {
			t.Errorf("parseProxyLine(%q) comment=%v want %v", c.raw, comment, c.comment)
		}
		if c.comment {
			continue
		}
		if u != c.wantURL {
			t.Errorf("parseProxyLine(%q) url=%q want %q", c.raw, u, c.wantURL)
		}
		if name != c.wantName {
			t.Errorf("parseProxyLine(%q) name=%q want %q", c.raw, name, c.wantName)
		}
	}
}

// 批量导入要能识别非法行并逐行报告，而不是整批失败。
func TestProxyPoolBulkReportsInvalidLines(t *testing.T) {
	m, err := NewManager(tempDir(t), "")
	if err != nil {
		t.Skipf("postgres unavailable (%v)", err)
	}
	defer m.Close()
	s := &Server{m: m}
	// 本用例测行解析；socks5 在 v0.6.32 起默认被 scheme 策略拒绝，这里显式开启。
	oldAllow, hadAllow, _ := m.pg.GetSetting(settingProxyAllowSocks5)
	if err := m.pg.SetSetting(settingProxyAllowSocks5, "true"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if hadAllow {
			_ = m.pg.SetSetting(settingProxyAllowSocks5, oldAllow)
		} else {
			_ = m.pg.SetSetting(settingProxyAllowSocks5, "false")
		}
	})

	body, _ := json.Marshal(map[string]any{
		"text": "# 注释\n1.2.3.4:8080\nsocks5://u:p@5.6.7.8:1080\n",
		"mode": "replace",
		// 本用例只测「行解析/非法行报告」，关掉导入验活（否则这些假地址会被判不可用丢弃）。
		"verify": false,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/proxypool/bulk", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.proxyPoolBulk(w, req)
	if w.Code != 200 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var res struct {
		Added   int `json:"added"`
		Invalid []struct {
			Line   int    `json:"line"`
			Reason string `json:"reason"`
		} `json:"invalid"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Added != 2 {
		t.Errorf("added=%d want 2", res.Added)
	}
	// 「not a url at all」含空格且不是合法 host:port，应被判为非法行。
	bad := "not a url at all"
	body, _ = json.Marshal(map[string]any{"text": bad, "mode": "replace", "verify": false})
	req = httptest.NewRequest(http.MethodPost, "/api/proxypool/bulk", bytes.NewReader(body))
	w = httptest.NewRecorder()
	s.proxyPoolBulk(w, req)
	if w.Code != 200 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	res.Invalid = nil
	res.Added = 0
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Invalid) != 1 {
		t.Fatalf("invalid=%+v want 1 条", res.Invalid)
	}
	if res.Invalid[0].Line != 1 {
		t.Errorf("line=%d want 1", res.Invalid[0].Line)
	}
	if res.Invalid[0].Reason == "" {
		t.Error("非法行必须带原因")
	}
}

// 代理池的连通性验证：好代理判可用，坏代理判不可用。
// 用一个本地 HTTP 代理当"好代理"，探测目标也用本地服务——不依赖外网。
func TestProxyPoolCheckDistinguishesGoodAndBad(t *testing.T) {
	m, err := NewManager(tempDir(t), "")
	if err != nil {
		t.Skipf("postgres unavailable (%v)", err)
	}
	defer m.Close()
	s := &Server{m: m}

	// 目标服务
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ip":"203.0.113.9"}`))
	}))
	defer target.Close()
	// 好代理：把请求原样转发到目标
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, _ := url.Parse(target.URL)
		r.URL.Scheme, r.URL.Host = u.Scheme, u.Host
		w.Write([]byte(`{"ip":"203.0.113.9"}`))
	}))
	defer good.Close()

	entries := []map[string]any{
		{"url": good.URL, "name": "good", "enabled": true, "weight": 1},
		{"url": "http://127.0.0.1:1", "name": "bad", "enabled": true, "weight": 1},
	}
	body, _ := json.Marshal(map[string]any{"entries": entries, "strategy": "round_robin"})
	req := httptest.NewRequest(http.MethodPut, "/api/proxypool", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.proxyPoolPut(w, req)
	if w.Code != 200 {
		t.Fatalf("put status=%d body=%s", w.Code, w.Body.String())
	}

	body, _ = json.Marshal(map[string]any{
		"target": target.URL, "break_on_fail": true, "timeout_seconds": 3,
	})
	req = httptest.NewRequest(http.MethodPost, "/api/proxypool/check", bytes.NewReader(body))
	w = httptest.NewRecorder()
	s.proxyPoolCheck(w, req)
	if w.Code != 200 {
		t.Fatalf("check status=%d body=%s", w.Code, w.Body.String())
	}
	var res struct {
		Results []struct {
			URL string `json:"url"`
			OK  bool   `json:"ok"`
		} `json:"results"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Results) != 2 {
		t.Fatalf("results=%d want 2", len(res.Results))
	}
	okCount := 0
	for _, r := range res.Results {
		if r.OK {
			okCount++
		}
	}
	if okCount != 1 {
		t.Errorf("可用=%d want 1（好代理应通、坏代理应不通）", okCount)
	}
}

// 导入验活（v0.6.26）：连不通的代理不导入、逐行给原因；可用的正常入库。
func TestProxyPoolBulkVerifiesAndDropsUnreachable(t *testing.T) {
	m, err := NewManager(tempDir(t), "")
	if err != nil {
		t.Skipf("postgres unavailable (%v)", err)
	}
	defer m.Close()
	s := &Server{m: m}

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ip":"203.0.113.9"}`))
	}))
	defer target.Close()
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ip":"203.0.113.9"}`))
	}))
	defer good.Close()
	// 探针目标改成测试服务，避免依赖外网。
	prevTarget, _, _ := m.pg.GetSetting(settingProxyCheckTarget)
	if err := m.pg.SetSetting(settingProxyCheckTarget, target.URL); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if prevTarget == "" {
			_, _ = m.pg.Exec(`DELETE FROM settings WHERE key=$1`, settingProxyCheckTarget)
		} else {
			_ = m.pg.SetSetting(settingProxyCheckTarget, prevTarget)
		}
	})

	body, _ := json.Marshal(map[string]any{
		"text":                   good.URL + "\nhttp://127.0.0.1:1\n",
		"mode":                   "replace",
		"verify":                 true,
		"verify_timeout_seconds": 2,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/proxypool/bulk", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.proxyPoolBulk(w, req)
	if w.Code != 200 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var res struct {
		Added    int  `json:"added"`
		Total    int  `json:"total"`
		Verified bool `json:"verified"`
		Rejected []struct {
			Line   int    `json:"line"`
			Reason string `json:"reason"`
		} `json:"rejected"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if !res.Verified {
		t.Fatalf("verified=false（验活应执行）body=%s", w.Body.String())
	}
	if res.Added != 1 || res.Total != 1 {
		t.Fatalf("added=%d total=%d want 1/1（坏代理应被丢弃）", res.Added, res.Total)
	}
	if len(res.Rejected) != 1 || res.Rejected[0].Reason == "" {
		t.Fatalf("rejected=%+v want 1 条带原因", res.Rejected)
	}
	st := s.loadProxyPool()
	if len(st.Entries) != 1 || st.Entries[0].URL != good.URL {
		t.Fatalf("entries=%+v want 只留好代理", st.Entries)
	}
}

// 总开关：关闭后持久化为 false，开启后恢复；默认（无该键）为开启。
func TestProxyPoolEnableTogglePersists(t *testing.T) {
	m, err := NewManager(tempDir(t), "")
	if err != nil {
		t.Skipf("postgres unavailable (%v)", err)
	}
	defer m.Close()
	s := &Server{m: m}
	t.Cleanup(func() { _, _ = m.pg.Exec(`DELETE FROM settings WHERE key=$1`, settingProxyPoolEnabled) })

	put := func(enabled bool) map[string]any {
		body, _ := json.Marshal(map[string]any{
			"entries":  []map[string]any{{"url": "http://127.0.0.1:9", "name": "x", "enabled": true, "weight": 1}},
			"strategy": "round_robin", "enabled": enabled,
		})
		req := httptest.NewRequest(http.MethodPut, "/api/proxypool", bytes.NewReader(body))
		w := httptest.NewRecorder()
		s.proxyPoolPut(w, req)
		if w.Code != 200 {
			t.Fatalf("put status=%d body=%s", w.Code, w.Body.String())
		}
		var out map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return out
	}
	if got := put(false)["enabled"]; got != false {
		t.Fatalf("enabled=%v want false", got)
	}
	if st := s.loadProxyPool(); st.Enabled {
		t.Fatal("persisted enabled=true want false")
	}
	if got := put(true)["enabled"]; got != true {
		t.Fatalf("enabled=%v want true", got)
	}
}

// 漏洞情报的错误提示：页面上"坏了因为啥坏的看不到"是原问题，
// 这里保证常见错误都能翻成可执行的建议。
func TestVulnIntelErrorHint(t *testing.T) {
	cases := []struct {
		err  string
		want string
	}{
		{"kev feed status 404: <!DOCTYPE html>", "404"},
		{"nvd api decode: context deadline exceeded", "超时"},
		{"dial tcp: lookup www.cisa.gov: no such host", "DNS"},
		{"proxyconnect tcp: connection refused", "代理"},
		{"status 429: rate limit exceeded", "限速"},
	}
	for _, c := range cases {
		hint := vulnIntelErrorHint(c.err)
		if hint == "" {
			t.Errorf("vulnIntelErrorHint(%q) 为空", c.err)
			continue
		}
		if !bytes.Contains([]byte(hint), []byte(c.want)) {
			t.Errorf("vulnIntelErrorHint(%q)=%q，期望包含 %q", c.err, hint, c.want)
		}
	}
	if vulnIntelErrorHint("") != "" {
		t.Error("无错误时不应给建议")
	}
	_ = time.Second
}

// v0.8.0：socks5 上游默认允许（Clash/mihomo 混合端常见）。显式 false 时才拒绝。
func TestProxyPoolBulkAllowsSocks5ByDefault(t *testing.T) {
	m, err := NewManager(tempDir(t), "")
	if err != nil {
		t.Skipf("postgres unavailable (%v)", err)
	}
	defer m.Close()
	s := &Server{m: m}
	old, had, _ := m.pg.GetSetting(settingProxyAllowSocks5)
	if err := m.pg.SetSetting(settingProxyAllowSocks5, "true"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if had {
			_ = m.pg.SetSetting(settingProxyAllowSocks5, old)
		}
	})
	body, _ := json.Marshal(map[string]any{
		"text": "socks5://u:p@5.6.7.8:1080\n", "mode": "replace", "verify": false,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/proxypool/bulk", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.proxyPoolBulk(w, req)
	if w.Code != 200 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var res struct {
		Added   int `json:"added"`
		Invalid []struct {
			Reason string `json:"reason"`
		} `json:"invalid"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Added != 1 || len(res.Invalid) != 0 {
		t.Fatalf("socks5 应默认允许: added=%d invalid=%+v", res.Added, res.Invalid)
	}
}

func TestProxyPoolBulkRejectsSocks5WhenDisabled(t *testing.T) {
	m, err := NewManager(tempDir(t), "")
	if err != nil {
		t.Skipf("postgres unavailable (%v)", err)
	}
	defer m.Close()
	s := &Server{m: m}
	old, had, _ := m.pg.GetSetting(settingProxyAllowSocks5)
	if err := m.pg.SetSetting(settingProxyAllowSocks5, "false"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if had {
			_ = m.pg.SetSetting(settingProxyAllowSocks5, old)
		}
	})
	body, _ := json.Marshal(map[string]any{
		"text": "socks5://u:p@5.6.7.8:1080\n", "mode": "replace", "verify": false,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/proxypool/bulk", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.proxyPoolBulk(w, req)
	if w.Code != 200 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var res struct {
		Added   int `json:"added"`
		Invalid []struct {
			Reason string `json:"reason"`
		} `json:"invalid"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Added != 0 || len(res.Invalid) != 1 {
		t.Fatalf("socks5 应被拒绝: added=%d invalid=%+v", res.Added, res.Invalid)
	}
	if !strings.Contains(res.Invalid[0].Reason, "socks5") {
		t.Fatalf("拒绝原因应说明 socks5: %s", res.Invalid[0].Reason)
	}
}

// 空列表保存会静默清空整池：必须显式 confirm_clear，防止页面状态过期误清空。
func TestProxyPoolPutRequiresConfirmToClear(t *testing.T) {
	m, err := NewManager(tempDir(t), "")
	if err != nil {
		t.Skipf("postgres unavailable (%v)", err)
	}
	defer m.Close()
	s := &Server{m: m}

	seed, _ := json.Marshal(map[string]any{
		"entries":  []map[string]any{{"url": "http://127.0.0.1:9", "name": "keep", "enabled": true, "weight": 1}},
		"strategy": "round_robin",
	})
	req := httptest.NewRequest(http.MethodPut, "/api/proxypool", bytes.NewReader(seed))
	w := httptest.NewRecorder()
	s.proxyPoolPut(w, req)
	if w.Code != 200 {
		t.Fatalf("seed status=%d body=%s", w.Code, w.Body.String())
	}

	empty, _ := json.Marshal(map[string]any{"entries": []any{}, "strategy": "round_robin"})
	req = httptest.NewRequest(http.MethodPut, "/api/proxypool", bytes.NewReader(empty))
	w = httptest.NewRecorder()
	s.proxyPoolPut(w, req)
	if w.Code != 400 {
		t.Fatalf("未确认清空应 400，得到 %d body=%s", w.Code, w.Body.String())
	}
	if got := len(s.loadProxyPool().Entries); got != 1 {
		t.Fatalf("拒绝后池应保持 1 条，得到 %d", got)
	}

	// 批量导入 replace + 空文本同样不能清空。
	bulk, _ := json.Marshal(map[string]any{"text": "   ", "mode": "replace"})
	req = httptest.NewRequest(http.MethodPost, "/api/proxypool/bulk", bytes.NewReader(bulk))
	w = httptest.NewRecorder()
	s.proxyPoolBulk(w, req)
	if w.Code != 400 {
		t.Fatalf("空文本替换导入应 400，得到 %d body=%s", w.Code, w.Body.String())
	}
	if got := len(s.loadProxyPool().Entries); got != 1 {
		t.Fatalf("批量拒绝后池应保持 1 条，得到 %d", got)
	}

	confirmed, _ := json.Marshal(map[string]any{"entries": []any{}, "strategy": "round_robin", "confirm_clear": true})
	req = httptest.NewRequest(http.MethodPut, "/api/proxypool", bytes.NewReader(confirmed))
	w = httptest.NewRecorder()
	s.proxyPoolPut(w, req)
	if w.Code != 200 {
		t.Fatalf("确认清空应 200，得到 %d body=%s", w.Code, w.Body.String())
	}
}

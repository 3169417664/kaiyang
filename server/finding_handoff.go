package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/3169417664/kaiyang/db"
	actool "github.com/Autumn-27/norma/tool"
)

// 内网渗透派发的资产键与去重工具。v1 以「IP > 根域 > 域 > URL host」为键，
// 同一主机的不同端口/路径合并到同一子任务。

func dispatchAssetKey(a *db.Asset) string {
	if a == nil {
		return ""
	}
	if strings.TrimSpace(a.IP) != "" {
		return strings.TrimSpace(a.IP)
	}
	for _, v := range []string{a.RootDomain, a.Domain} {
		if strings.TrimSpace(v) != "" {
			return strings.ToLower(strings.TrimSpace(v))
		}
	}
	if u, err := url.Parse(a.URL); err == nil && u.Hostname() != "" {
		return strings.ToLower(u.Hostname())
	}
	return ""
}

func (s *Server) toolCheckInternalAssetTask() actool.CoreTool {
	return wrTool("check_internal_asset_task",
		"按 Finding 实际关联的 IP/域名检查是否已创建内网渗透子任务；端口不同的同一主机使用同一资产键。必须在 spawn_task 前调用。",
		objSchema(map[string]any{
			"finding_id": map[string]any{"type": "integer", "description": "findings.id"},
		}, "finding_id"),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			var a struct {
				FindingID int64 `json:"finding_id"`
			}
			if err := json.Unmarshal(in, &a); err != nil {
				return actool.Errorf("参数无效"), nil
			}
			f, err := s.m.pg.GetFinding(a.FindingID)
			if err != nil || f == nil {
				return actool.Errorf("finding 不存在"), nil
			}
			var assets []*db.Asset
			if len(f.AssetIDs) > 0 {
				assets, _ = s.m.assets.GetByIDs(f.AssetIDs)
			}
			keys := make([]string, 0, len(assets))
			for _, as := range assets {
				if k := dispatchAssetKey(as); k != "" {
					keys = append(keys, k)
				}
			}
			if len(keys) == 0 {
				return jsonResult(map[string]any{"exists": false, "asset_keys": []string{}, "hint": "finding 没有可解析的 IP/域名资产"})
			}
			existing := map[string]any{}
			for _, k := range keys {
				if tid, fid, ok, _ := s.m.pg.GetTaskDispatchTarget(k); ok {
					existing[k] = map[string]any{"task_id": tid, "finding_id": fid}
				}
			}
			return jsonResult(map[string]any{"exists": len(existing) > 0, "asset_keys": keys, "existing": existing})
		})
}

func (s *Server) toolAppendFindingToTask() actool.CoreTool {
	return wrTool("append_finding_to_task",
		"把一条 Finding 追加到已有任务（通常是内网渗透子任务），并向目标任务写一条 hint。按资产键去重，同一资产只追加一次。",
		objSchema(map[string]any{
			"finding_id": map[string]any{"type": "integer", "description": "findings.id"},
			"task_id":    map[string]any{"type": "string", "description": "目标任务 id"},
			"reason":     strParam("可选：追加原因"),
		}, "finding_id", "task_id"),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			var a struct {
				FindingID int64  `json:"finding_id"`
				TaskID    string `json:"task_id"`
				Reason    string `json:"reason"`
			}
			if err := json.Unmarshal(in, &a); err != nil {
				return actool.Errorf("参数无效"), nil
			}
			target, ok := s.m.Task(strings.TrimSpace(a.TaskID))
			if !ok {
				return actool.Errorf("目标任务不存在"), nil
			}
			f, err := s.m.pg.GetFinding(a.FindingID)
			if err != nil || f == nil {
				return actool.Errorf("finding 不存在"), nil
			}
			var assets []*db.Asset
			if len(f.AssetIDs) > 0 {
				assets, _ = s.m.assets.GetByIDs(f.AssetIDs)
			}
			keys := make([]string, 0, len(assets))
			for _, as := range assets {
				if k := dispatchAssetKey(as); k != "" {
					keys = append(keys, k)
				}
			}
			if len(keys) == 0 {
				return actool.Errorf("finding 没有可解析的 IP/域名资产"), nil
			}
			added := 0
			for _, k := range keys {
				if _, _, exists, _ := s.m.pg.GetTaskDispatchTarget(k); exists {
					continue
				}
				tid, _ := strconv.ParseInt(target.ID, 10, 64)
				if err := s.m.pg.RecordTaskDispatchTarget(k, tid, f.ID); err != nil {
					continue
				}
				added++
			}
			summary := fmt.Sprintf("[跨任务] 来自任务 #%d 的漏洞：%s（%s/%s）%s",
				valueOrZero(f.TaskID), f.Name, f.VulnClass, f.Severity, strings.TrimSpace(a.Reason))
			_, _ = target.Store.AddNode("hint", map[string]any{"summary": summary, "source_finding_id": f.ID}, 5, "active", "vuln_dispatcher", nil)
			return jsonResult(map[string]any{"ok": true, "added": added, "task_id": target.ID})
		})
}

func valueOrZero(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

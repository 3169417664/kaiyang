package server

import (
	"context"
	"database/sql"
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

func normalizeAssetKey(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func dispatchAssetKey(a *db.Asset) string {
	if a == nil {
		return ""
	}
	if strings.TrimSpace(a.IP) != "" {
		return normalizeAssetKey(a.IP)
	}
	for _, v := range []string{a.RootDomain, a.Domain} {
		if strings.TrimSpace(v) != "" {
			return normalizeAssetKey(v)
		}
	}
	if u, err := url.Parse(a.URL); err == nil && u.Hostname() != "" {
		return normalizeAssetKey(u.Hostname())
	}
	return ""
}

// findingDispatchAssetKeys 解析 Finding 实际关联资产的规范化键。
// targetAsset 非空时只保留与它匹配的那个资产（多主机 Finding 必须显式指定）。
func (s *Server) findingDispatchAssetKeys(findingID int64, targetAsset string) ([]string, error) {
	f, err := s.m.pg.GetFinding(findingID)
	if err != nil {
		return nil, fmt.Errorf("读取 Finding 失败: %w", err)
	}
	if f == nil {
		return nil, fmt.Errorf("Finding 不存在")
	}
	var assets []*db.Asset
	if len(f.AssetIDs) > 0 {
		assets, _ = s.m.assets.GetByIDs(f.AssetIDs)
	}
	want := normalizeAssetKey(targetAsset)
	if strings.Contains(want, "://") {
		if u, err := url.Parse(want); err == nil {
			want = normalizeAssetKey(u.Hostname())
		}
	} else if host, port, err := splitHostPort(want); err == nil && port != "" {
		want = normalizeAssetKey(host)
	}
	seen := map[string]struct{}{}
	keys := make([]string, 0, len(assets))
	for _, as := range assets {
		key := dispatchAssetKey(as)
		if key == "" {
			continue
		}
		if want != "" && !assetMatchesKey(as, key, want) {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	if want != "" && len(keys) == 0 {
		return nil, fmt.Errorf("target_asset %q 不属于 Finding #%d 的资产", targetAsset, findingID)
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("Finding 没有可解析的 IP/域名资产")
	}
	return keys, nil
}

func splitHostPort(raw string) (host, port string, err error) {
	if i := strings.LastIndexByte(raw, ':'); i > 0 && !strings.Contains(raw[i+1:], ":") {
		return raw[:i], raw[i+1:], nil
	}
	return raw, "", fmt.Errorf("no port")
}

func assetMatchesKey(a *db.Asset, key, want string) bool {
	if key == want {
		return true
	}
	for _, v := range []string{a.IP, a.RootDomain, a.Domain} {
		if normalizeAssetKey(v) != "" && normalizeAssetKey(v) == want {
			return true
		}
	}
	if u, err := url.Parse(a.URL); err == nil && normalizeAssetKey(u.Hostname()) == want {
		return true
	}
	return false
}

func findingHandoffMarker(findingID int64) string {
	return fmt.Sprintf("[dispatcher_finding_id=%d]", findingID)
}

func clipFindingHandoff(text string, maxRunes int) string {
	r := []rune(strings.TrimSpace(text))
	if len(r) <= maxRunes {
		return string(r)
	}
	return string(r[:maxRunes]) + "…"
}

func findingHandoffText(f *db.DBFinding) string {
	var b strings.Builder
	b.WriteString(findingHandoffMarker(f.ID))
	b.WriteString("\n同一资产新增 Finding，请纳入当前内网渗透规划，不要重复创建子任务。\n")
	if f.TaskID != nil {
		fmt.Fprintf(&b, "来源任务：#%d\n", *f.TaskID)
	}
	fmt.Fprintf(&b, "Finding：#%d · %s · %s\n", f.ID, f.VulnClass, f.Severity)
	if name := clipFindingHandoff(f.Name, 500); name != "" {
		b.WriteString("名称：" + name + "\n")
	}
	b.WriteString("摘要：" + clipFindingHandoff(f.Summary, 4000) + "\n")
	if evidence := clipFindingHandoff(f.Evidence, 8000); evidence != "" {
		b.WriteString("证据：\n" + evidence)
	}
	return b.String()
}

func (s *Server) toolCheckInternalAssetTask() actool.CoreTool {
	return wrTool("check_internal_asset_task",
		"按 Finding 实际关联的 IP/域名检查是否已创建内网渗透子任务；端口不同的同一主机使用同一资产键。必须在 spawn_task 前调用。",
		objSchema(map[string]any{
			"finding_id":   map[string]any{"type": "integer", "description": "findings.id"},
			"target_asset": strParam("可选：多主机 Finding 的精确资产 IP/域名/URL"),
		}, "finding_id"),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			var a struct {
				FindingID   int64  `json:"finding_id"`
				TargetAsset string `json:"target_asset"`
			}
			if err := json.Unmarshal(in, &a); err != nil || a.FindingID <= 0 {
				return actool.Errorf("finding_id 必填"), nil
			}
			keys, err := s.findingDispatchAssetKeys(a.FindingID, a.TargetAsset)
			if err != nil {
				return actool.Errorf(err.Error()), nil
			}
			existing := map[string]any{}
			for _, k := range keys {
				if tid, fid, ok, _ := s.m.pg.GetTaskDispatchTarget(k); ok {
					existing[k] = map[string]any{"task_id": strconv.FormatInt(tid, 10), "finding_id": fid}
				}
			}
			return jsonResult(map[string]any{"exists": len(existing) > 0, "asset_keys": keys, "existing": existing})
		})
}

func (s *Server) toolAppendFindingToTask() actool.CoreTool {
	return wrTool("append_finding_to_task",
		"把 Finding 的内容和证据补充到负责同一资产的内网渗透任务。参数 task_id=内网任务 id，finding_id=Finding id；多主机 Finding 必须传 target_asset。"+
			"目标必须已登记相同资产；幂等：重复追加返回 already_appended。写入 hint + fact 链（fact 锚定资产并 derived_from 到 hint），终态任务会新增跟进目标并恢复。",
		objSchema(map[string]any{
			"task_id":      strParam("负责同一资产的内网渗透任务 id"),
			"finding_id":   map[string]any{"type": "integer", "description": "要补充的 Finding id"},
			"target_asset": strParam("可选：多主机 Finding 的精确资产 IP/域名/URL"),
			"reason":       strParam("可选：追加原因"),
		}, "task_id", "finding_id"),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			var a struct {
				TaskID      string          `json:"task_id"`
				FindingID   json.RawMessage `json:"finding_id"`
				TargetAsset string          `json:"target_asset"`
				Reason      string          `json:"reason"`
			}
			_ = json.Unmarshal(in, &a)
			taskID := strings.TrimSpace(a.TaskID)
			findingID := parseProfileID(a.FindingID)
			if taskID == "" || findingID <= 0 {
				return actool.Errorf("task_id 和有效的 finding_id 为必填"), nil
			}
			target, ok := s.m.Task(taskID)
			if !ok || target == nil {
				return actool.Errorf("子任务不存在: " + taskID), nil
			}
			if target.CategoryName != "" && target.CategoryName != "内网渗透" {
				return actool.Errorf("目标任务不是内网渗透子任务"), nil
			}
			finding, err := s.m.pg.GetFinding(findingID)
			if err != nil {
				return actool.Errorf("读取 Finding 失败: " + err.Error()), nil
			}
			if finding == nil {
				return actool.Errorf("Finding 不存在"), nil
			}
			keys, err := s.findingDispatchAssetKeys(findingID, a.TargetAsset)
			if err != nil {
				return actool.Errorf(err.Error()), nil
			}
			// 目标必须登记了同一资产；若资产键属于别的任务，拒绝跨任务抢写。
			matchedAsset := ""
			for _, key := range keys {
				tid, _, exists, qerr := s.m.pg.GetTaskDispatchTarget(key)
				if qerr != nil {
					return actool.Errorf("检查资产派发记录失败: " + qerr.Error()), nil
				}
				if !exists {
					continue
				}
				if strconv.FormatInt(tid, 10) != target.ID {
					return actool.Errorf(fmt.Sprintf("资产 %s 已登记在任务 #%d，不能追加到任务 %s", key, tid, target.ID)), nil
				}
				matchedAsset = key
				break
			}
			if matchedAsset == "" {
				return actool.Errorf("目标任务未登记该 Finding 的资产，不能追加"), nil
			}

			marker := findingHandoffMarker(findingID)
			var existingHintID int64
			err = s.m.pg.QueryRow(`SELECT id FROM exploration_nodes
				WHERE exploration_id=$1 AND kind='hint' AND payload->>'summary' LIKE $2
				ORDER BY id LIMIT 1`, target.ExpID, marker+"%").Scan(&existingHintID)
			if err == nil {
				return jsonResult(map[string]any{
					"status": "already_appended", "task_id": target.ID,
					"finding_id": findingID, "hint_id": existingHintID, "asset_key": matchedAsset,
				})
			}
			if err != sql.ErrNoRows {
				return actool.Errorf("检查 Finding 追加记录失败: " + err.Error()), nil
			}

			var goalID int64
			if isTerminalStatus(target.lifecycleSnapshot().Status) {
				openGoal, goalErr := target.Store.HasOpenGoal()
				if goalErr != nil {
					return actool.Errorf(fmt.Sprintf("检查目标任务失败: %v", goalErr)), nil
				}
				if !openGoal {
					goalText := fmt.Sprintf("%s 评估并处理同一资产新增 Finding #%d，纳入内网渗透规划。", marker, findingID)
					goalID, goalErr = target.Store.AddGoal(map[string]any{"text": goalText, "vulnclass": finding.VulnClass}, "vuln_dispatcher")
					if goalErr != nil {
						return actool.Errorf(fmt.Sprintf("新增 Finding 目标失败: %v", goalErr)), nil
					}
					target.NotifyGoal([]string{goalText})
				}
			}

			text := findingHandoffText(finding)
			hintID, err := target.Store.AddNode("hint", map[string]any{
				"summary": text, "source_finding_id": finding.ID, "source_task_id": valueOrZero(finding.TaskID),
			}, 5, "active", "vuln_dispatcher", nil)
			if err != nil {
				return actool.Errorf("写入 hint 失败: " + err.Error()), nil
			}
			// fact 链：把 Finding 的结构化证据落成目标任务的 fact 节点，锚定资产并
			// derived_from 到 hint。这样目标 planner 的态势里既有战略提示，也有可
			// 复核的事实与血缘，而不是只有一句转述。
			factID, err := target.Store.AddNode("fact", map[string]any{
				"summary":           fmt.Sprintf("[跨任务] Finding #%d · %s · %s · %s", finding.ID, finding.VulnClass, finding.Severity, clipFindingHandoff(finding.Name, 300)),
				"evidence":          clipFindingHandoff(finding.Evidence, 8000),
				"source_finding_id": finding.ID,
				"source_task_id":    valueOrZero(finding.TaskID),
				"dispatch_asset":    matchedAsset,
				"confidence":        "observed",
				"reason":            strings.TrimSpace(a.Reason),
			}, 5, "active", "vuln_dispatcher", finding.AssetIDs)
			if err != nil {
				return actool.Errorf("写入 fact 失败: " + err.Error()), nil
			}
			if err := target.Store.Link(hintID, db.RelDerivedFrom, factID); err != nil {
				return actool.Errorf("写入 hint→fact 血缘失败: " + err.Error()), nil
			}
			if origin, _ := target.Store.OriginFactID(); origin > 0 && origin != factID {
				_ = target.Store.Link(origin, db.RelDerivedFrom, factID)
			}
			if err := s.m.pg.RecordTaskDispatchTarget(matchedAsset, mustInt64(target.ID), finding.ID); err != nil {
				return actool.Errorf("登记资产派发记录失败: " + err.Error()), nil
			}
			_, _ = target.Store.AppendActivity(db.Activity{
				NodeID: &hintID, Worker: "vuln_dispatcher", Kind: "finding_handoff",
				Summary: fmt.Sprintf("Finding #%d 已补充到同资产内网子任务", finding.ID),
				Detail:  fmt.Sprintf("task_id=%s finding_id=%d hint_id=%d fact_id=%d goal_id=%d asset=%s", target.ID, finding.ID, hintID, factID, goalID, matchedAsset),
			})
			target.NotifyHint([]string{firstLine(text, 200)})
			// 终态任务：新增目标后按共享准入路径恢复，planner 才能消费 hint/fact。
			if goalID > 0 {
				_, _ = s.admitTask(target, "resume")
			}
			return jsonResult(map[string]any{
				"status": "appended", "task_id": target.ID, "finding_id": finding.ID,
				"hint_id": hintID, "fact_id": factID, "goal_id": goalID, "asset_key": matchedAsset,
			})
		})
}

func mustInt64(s string) int64 {
	v, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return v
}

func valueOrZero(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

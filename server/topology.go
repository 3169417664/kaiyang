package server

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// 任务内网拓扑：节点 / 边 / 分组 / 便签。数据按 task_id 隔离。
// 拓扑只对「内网渗透」分类的任务开放，其它分类一律 400。

const topologyCategoryNKey = "内网渗透"

type topoNode struct {
	IP       string  `json:"ip"`
	Hostname string  `json:"hostname"`
	Label    string  `json:"display_label"`
	GroupID  *int64  `json:"group_id,omitempty"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Notes    string  `json:"notes"`
}
type topoEdge struct {
	ID    int64  `json:"id"`
	Src   string `json:"src_ip"`
	Dst   string `json:"dst_ip"`
	Label string `json:"label"`
}
type topoGroup struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}
type topoNote struct {
	ID      int64   `json:"id"`
	Content string  `json:"content"`
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
}

// topologyTask resolves the task id and enforces the 内网渗透 category gate.
// Missing tasks are 404; tasks in any other category are 400.
func (s *Server) topologyTask(w http.ResponseWriter, r *http.Request) (int64, bool) {
	t, ok := s.m.Task(r.PathValue("id"))
	if !ok || t == nil {
		writeErr(w, 404, "task not found")
		return 0, false
	}
	id, err := strconv.ParseInt(t.ID, 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, 400, "invalid task id")
		return 0, false
	}
	var nkey string
	err = s.m.pg.QueryRow(`SELECT COALESCE(c.nkey,'') FROM tasks t
		LEFT JOIN task_categories c ON c.id=t.category_id WHERE t.id=$1`, id).Scan(&nkey)
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 404, "task not found")
		return 0, false
	}
	if err != nil {
		writeInternalError(w, "topology task category", err)
		return 0, false
	}
	if nkey != topologyCategoryNKey {
		writeErr(w, 400, "拓扑仅支持内网渗透分类任务")
		return 0, false
	}
	return id, true
}

func parseTopologyID(raw string) (int64, bool) {
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	return id, err == nil && id > 0
}

// topologyGroupInTask reports whether gid belongs to the given task. Group ids
// are globally allocated, so without this check a node could point at another
// task's group.
func (s *Server) topologyGroupInTask(tid, gid int64) (bool, error) {
	var ok bool
	err := s.m.pg.QueryRow(`SELECT EXISTS(SELECT 1 FROM task_topology_groups
		WHERE task_id=$1 AND id=$2)`, tid, gid).Scan(&ok)
	return ok, err
}

func (s *Server) topologyNodeExists(tid int64, ip string) (bool, error) {
	var ok bool
	err := s.m.pg.QueryRow(`SELECT EXISTS(SELECT 1 FROM task_topology_nodes
		WHERE task_id=$1 AND ip=$2)`, tid, ip).Scan(&ok)
	return ok, err
}

func (s *Server) getTopology(w http.ResponseWriter, r *http.Request) {
	tid, ok := s.topologyTask(w, r)
	if !ok {
		return
	}
	pg := s.m.pg
	groups := []topoGroup{}
	rows, err := pg.Query(`SELECT id,name,color FROM task_topology_groups WHERE task_id=$1 ORDER BY id`, tid)
	if err != nil {
		writeInternalError(w, "topology groups", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var g topoGroup
		if err := rows.Scan(&g.ID, &g.Name, &g.Color); err != nil {
			writeInternalError(w, "topology groups scan", err)
			return
		}
		groups = append(groups, g)
	}
	nodes := []topoNode{}
	nrows, err := pg.Query(`SELECT ip,hostname,display_label,group_id,x,y,notes
		FROM task_topology_nodes WHERE task_id=$1 ORDER BY ip`, tid)
	if err != nil {
		writeInternalError(w, "topology nodes", err)
		return
	}
	defer nrows.Close()
	for nrows.Next() {
		var n topoNode
		if err := nrows.Scan(&n.IP, &n.Hostname, &n.Label, &n.GroupID, &n.X, &n.Y, &n.Notes); err != nil {
			writeInternalError(w, "topology nodes scan", err)
			return
		}
		nodes = append(nodes, n)
	}
	edges := []topoEdge{}
	erows, err := pg.Query(`SELECT id,src_ip,dst_ip,label FROM task_topology_edges
		WHERE task_id=$1 ORDER BY id`, tid)
	if err != nil {
		writeInternalError(w, "topology edges", err)
		return
	}
	defer erows.Close()
	for erows.Next() {
		var e topoEdge
		if err := erows.Scan(&e.ID, &e.Src, &e.Dst, &e.Label); err != nil {
			writeInternalError(w, "topology edges scan", err)
			return
		}
		edges = append(edges, e)
	}
	notes := []topoNote{}
	trows, err := pg.Query(`SELECT id,content,x,y FROM task_topology_notes
		WHERE task_id=$1 ORDER BY id`, tid)
	if err != nil {
		writeInternalError(w, "topology notes", err)
		return
	}
	defer trows.Close()
	for trows.Next() {
		var n topoNote
		if err := trows.Scan(&n.ID, &n.Content, &n.X, &n.Y); err != nil {
			writeInternalError(w, "topology notes scan", err)
			return
		}
		notes = append(notes, n)
	}
	writeJSON(w, 200, map[string]any{"nodes": nodes, "edges": edges, "groups": groups, "notes": notes})
}

func (s *Server) upsertTopologyNode(w http.ResponseWriter, r *http.Request) {
	tid, ok := s.topologyTask(w, r)
	if !ok {
		return
	}
	var n topoNode
	if !decodeJSON(w, r, &n, maxSmallJSONBodyBytes) {
		return
	}
	n.IP = strings.TrimSpace(n.IP)
	if n.IP == "" {
		writeErr(w, 400, "ip 必填")
		return
	}
	if n.GroupID != nil {
		belongs, err := s.topologyGroupInTask(tid, *n.GroupID)
		if err != nil {
			writeInternalError(w, "topology node group check", err)
			return
		}
		if !belongs {
			writeErr(w, 400, "group_id 不存在")
			return
		}
	}
	_, err := s.m.pg.Exec(`INSERT INTO task_topology_nodes(task_id,ip,hostname,display_label,group_id,x,y,notes)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (task_id,ip) DO UPDATE SET hostname=EXCLUDED.hostname, display_label=EXCLUDED.display_label,
		  group_id=EXCLUDED.group_id, x=EXCLUDED.x, y=EXCLUDED.y, notes=EXCLUDED.notes, updated_at=now()`,
		tid, n.IP, n.Hostname, n.Label, n.GroupID, n.X, n.Y, n.Notes)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "ip": n.IP})
}

// patchTopologyNode performs a partial update. Only fields present in the JSON
// body are written, so dragging a node (x/y only) never clobbers hostname,
// label, notes or group membership. group_id:null explicitly clears the group.
func (s *Server) patchTopologyNode(w http.ResponseWriter, r *http.Request) {
	tid, ok := s.topologyTask(w, r)
	if !ok {
		return
	}
	ip := strings.TrimSpace(r.PathValue("ip"))
	if ip == "" {
		writeErr(w, 400, "ip 必填")
		return
	}
	var p struct {
		Hostname *string         `json:"hostname"`
		Label    *string         `json:"display_label"`
		GroupID  json.RawMessage `json:"group_id"`
		X        *float64        `json:"x"`
		Y        *float64        `json:"y"`
		Notes    *string         `json:"notes"`
	}
	if !decodeJSON(w, r, &p, maxSmallJSONBodyBytes) {
		return
	}
	sets := make([]string, 0, 8)
	args := []any{tid, ip}
	add := func(col string, v any) {
		args = append(args, v)
		sets = append(sets, fmt.Sprintf("%s=$%d", col, len(args)))
	}
	if p.Hostname != nil {
		add("hostname", *p.Hostname)
	}
	if p.Label != nil {
		add("display_label", *p.Label)
	}
	if p.X != nil {
		add("x", *p.X)
	}
	if p.Y != nil {
		add("y", *p.Y)
	}
	if p.Notes != nil {
		add("notes", *p.Notes)
	}
	if len(bytes.TrimSpace(p.GroupID)) > 0 {
		if string(bytes.TrimSpace(p.GroupID)) == "null" {
			sets = append(sets, "group_id=NULL")
		} else {
			var gid int64
			if err := json.Unmarshal(p.GroupID, &gid); err != nil || gid <= 0 {
				writeErr(w, 400, "group_id 非法")
				return
			}
			belongs, err := s.topologyGroupInTask(tid, gid)
			if err != nil {
				writeInternalError(w, "topology node group check", err)
				return
			}
			if !belongs {
				writeErr(w, 400, "group_id 不存在")
				return
			}
			add("group_id", gid)
		}
	}
	if len(sets) == 0 {
		writeJSON(w, 200, map[string]any{"ok": true})
		return
	}
	sets = append(sets, "updated_at=now()")
	res, err := s.m.pg.Exec(`UPDATE task_topology_nodes SET `+strings.Join(sets, ",")+
		` WHERE task_id=$1 AND ip=$2`, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeErr(w, 404, "node not found")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) deleteTopologyNode(w http.ResponseWriter, r *http.Request) {
	tid, ok := s.topologyTask(w, r)
	if !ok {
		return
	}
	ip := strings.TrimSpace(r.PathValue("ip"))
	if ip == "" {
		writeErr(w, 400, "ip 必填")
		return
	}
	_, _ = s.m.pg.Exec(`DELETE FROM task_topology_edges WHERE task_id=$1 AND (src_ip=$2 OR dst_ip=$2)`, tid, ip)
	_, _ = s.m.pg.Exec(`DELETE FROM task_topology_nodes WHERE task_id=$1 AND ip=$2`, tid, ip)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) addTopologyEdge(w http.ResponseWriter, r *http.Request) {
	tid, ok := s.topologyTask(w, r)
	if !ok {
		return
	}
	var e topoEdge
	if !decodeJSON(w, r, &e, maxSmallJSONBodyBytes) {
		return
	}
	e.Src = strings.TrimSpace(e.Src)
	e.Dst = strings.TrimSpace(e.Dst)
	if e.Src == "" || e.Dst == "" {
		writeErr(w, 400, "src_ip / dst_ip 必填")
		return
	}
	for _, ip := range []string{e.Src, e.Dst} {
		exists, err := s.topologyNodeExists(tid, ip)
		if err != nil {
			writeInternalError(w, "topology edge node check", err)
			return
		}
		if !exists {
			writeErr(w, 400, "引用的节点不存在: "+ip)
			return
		}
	}
	if err := s.m.pg.QueryRow(`INSERT INTO task_topology_edges(task_id,src_ip,dst_ip,label)
		VALUES ($1,$2,$3,$4) RETURNING id`, tid, e.Src, e.Dst, e.Label).Scan(&e.ID); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, e)
}

func (s *Server) patchTopologyEdge(w http.ResponseWriter, r *http.Request) {
	tid, ok := s.topologyTask(w, r)
	if !ok {
		return
	}
	id, ok := parseTopologyID(r.PathValue("eid"))
	if !ok {
		writeErr(w, 400, "invalid edge id")
		return
	}
	var p struct {
		Src   *string `json:"src_ip"`
		Dst   *string `json:"dst_ip"`
		Label *string `json:"label"`
	}
	if !decodeJSON(w, r, &p, maxSmallJSONBodyBytes) {
		return
	}
	// Repointing an edge requires both endpoints; a label-only patch leaves
	// endpoints untouched.
	if p.Src != nil || p.Dst != nil {
		if p.Src == nil || p.Dst == nil {
			writeErr(w, 400, "src_ip / dst_ip 需同时提供")
			return
		}
		src := strings.TrimSpace(*p.Src)
		dst := strings.TrimSpace(*p.Dst)
		if src == "" || dst == "" {
			writeErr(w, 400, "src_ip / dst_ip 必填")
			return
		}
		for _, ip := range []string{src, dst} {
			exists, err := s.topologyNodeExists(tid, ip)
			if err != nil {
				writeInternalError(w, "topology edge node check", err)
				return
			}
			if !exists {
				writeErr(w, 400, "引用的节点不存在: "+ip)
				return
			}
		}
		p.Src, p.Dst = &src, &dst
	}
	sets := make([]string, 0, 3)
	args := []any{tid, id}
	add := func(col string, v any) {
		args = append(args, v)
		sets = append(sets, fmt.Sprintf("%s=$%d", col, len(args)))
	}
	if p.Label != nil {
		add("label", *p.Label)
	}
	if p.Src != nil {
		add("src_ip", *p.Src)
		add("dst_ip", *p.Dst)
	}
	if len(sets) == 0 {
		writeJSON(w, 200, map[string]any{"ok": true})
		return
	}
	res, err := s.m.pg.Exec(`UPDATE task_topology_edges SET `+strings.Join(sets, ",")+
		` WHERE task_id=$1 AND id=$2`, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeErr(w, 404, "edge not found")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) deleteTopologyEdge(w http.ResponseWriter, r *http.Request) {
	tid, ok := s.topologyTask(w, r)
	if !ok {
		return
	}
	id, ok := parseTopologyID(r.PathValue("eid"))
	if !ok {
		writeErr(w, 400, "invalid edge id")
		return
	}
	_, _ = s.m.pg.Exec(`DELETE FROM task_topology_edges WHERE task_id=$1 AND id=$2`, tid, id)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) addTopologyGroup(w http.ResponseWriter, r *http.Request) {
	tid, ok := s.topologyTask(w, r)
	if !ok {
		return
	}
	var g topoGroup
	if !decodeJSON(w, r, &g, maxSmallJSONBodyBytes) {
		return
	}
	g.Name = strings.TrimSpace(g.Name)
	if g.Name == "" {
		writeErr(w, 400, "name 必填")
		return
	}
	if err := s.m.pg.QueryRow(`INSERT INTO task_topology_groups(task_id,name,color)
		VALUES ($1,$2,$3) RETURNING id`, tid, g.Name, g.Color).Scan(&g.ID); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, g)
}

func (s *Server) patchTopologyGroup(w http.ResponseWriter, r *http.Request) {
	tid, ok := s.topologyTask(w, r)
	if !ok {
		return
	}
	id, ok := parseTopologyID(r.PathValue("gid"))
	if !ok {
		writeErr(w, 400, "invalid group id")
		return
	}
	var p struct {
		Name  *string `json:"name"`
		Color *string `json:"color"`
	}
	if !decodeJSON(w, r, &p, maxSmallJSONBodyBytes) {
		return
	}
	sets := make([]string, 0, 3)
	args := []any{tid, id}
	add := func(col string, v any) {
		args = append(args, v)
		sets = append(sets, fmt.Sprintf("%s=$%d", col, len(args)))
	}
	if p.Name != nil {
		name := strings.TrimSpace(*p.Name)
		if name == "" {
			writeErr(w, 400, "name 必填")
			return
		}
		add("name", name)
	}
	if p.Color != nil {
		add("color", *p.Color)
	}
	if len(sets) == 0 {
		writeJSON(w, 200, map[string]any{"ok": true})
		return
	}
	res, err := s.m.pg.Exec(`UPDATE task_topology_groups SET `+strings.Join(sets, ",")+
		` WHERE task_id=$1 AND id=$2`, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeErr(w, 404, "group not found")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) deleteTopologyGroup(w http.ResponseWriter, r *http.Request) {
	tid, ok := s.topologyTask(w, r)
	if !ok {
		return
	}
	id, ok := parseTopologyID(r.PathValue("gid"))
	if !ok {
		writeErr(w, 400, "invalid group id")
		return
	}
	_, _ = s.m.pg.Exec(`UPDATE task_topology_nodes SET group_id=NULL WHERE task_id=$1 AND group_id=$2`, tid, id)
	_, _ = s.m.pg.Exec(`DELETE FROM task_topology_groups WHERE task_id=$1 AND id=$2`, tid, id)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) addTopologyNote(w http.ResponseWriter, r *http.Request) {
	tid, ok := s.topologyTask(w, r)
	if !ok {
		return
	}
	var n topoNote
	if !decodeJSON(w, r, &n, maxSmallJSONBodyBytes) {
		return
	}
	n.Content = strings.TrimSpace(n.Content)
	if n.Content == "" {
		writeErr(w, 400, "content 必填")
		return
	}
	if err := s.m.pg.QueryRow(`INSERT INTO task_topology_notes(task_id,content,x,y)
		VALUES ($1,$2,$3,$4) RETURNING id`, tid, n.Content, n.X, n.Y).Scan(&n.ID); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, n)
}

func (s *Server) patchTopologyNote(w http.ResponseWriter, r *http.Request) {
	tid, ok := s.topologyTask(w, r)
	if !ok {
		return
	}
	id, ok := parseTopologyID(r.PathValue("nid"))
	if !ok {
		writeErr(w, 400, "invalid note id")
		return
	}
	var p struct {
		Content *string  `json:"content"`
		X       *float64 `json:"x"`
		Y       *float64 `json:"y"`
	}
	if !decodeJSON(w, r, &p, maxSmallJSONBodyBytes) {
		return
	}
	sets := make([]string, 0, 3)
	args := []any{tid, id}
	if p.Content != nil {
		content := strings.TrimSpace(*p.Content)
		if content == "" {
			writeErr(w, 400, "content 必填")
			return
		}
		args = append(args, content)
		sets = append(sets, fmt.Sprintf("content=$%d", len(args)))
	}
	if p.X != nil {
		args = append(args, *p.X)
		sets = append(sets, fmt.Sprintf("x=$%d", len(args)))
	}
	if p.Y != nil {
		args = append(args, *p.Y)
		sets = append(sets, fmt.Sprintf("y=$%d", len(args)))
	}
	if len(sets) == 0 {
		writeJSON(w, 200, map[string]any{"ok": true})
		return
	}
	res, err := s.m.pg.Exec(`UPDATE task_topology_notes SET `+strings.Join(sets, ",")+
		` WHERE task_id=$1 AND id=$2`, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeErr(w, 404, "note not found")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) deleteTopologyNote(w http.ResponseWriter, r *http.Request) {
	tid, ok := s.topologyTask(w, r)
	if !ok {
		return
	}
	id, ok := parseTopologyID(r.PathValue("nid"))
	if !ok {
		writeErr(w, 400, "invalid note id")
		return
	}
	_, _ = s.m.pg.Exec(`DELETE FROM task_topology_notes WHERE task_id=$1 AND id=$2`, tid, id)
	writeJSON(w, 200, map[string]any{"ok": true})
}

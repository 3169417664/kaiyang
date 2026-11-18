package server

import (
	"net/http"
	"strconv"
	"strings"
)

// 任务内网拓扑：节点 / 边 / 分组 / 便签。数据按 task_id 隔离。

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

func (s *Server) taskIDForTopology(w http.ResponseWriter, r *http.Request) (int64, bool) {
	t, ok := s.m.Task(r.PathValue("id"))
	if !ok {
		writeErr(w, 404, "task not found")
		return 0, false
	}
	id, err := strconv.ParseInt(t.ID, 10, 64)
	if err != nil {
		writeErr(w, 400, "invalid task id")
		return 0, false
	}
	return id, true
}

func (s *Server) getTopology(w http.ResponseWriter, r *http.Request) {
	tid, ok := s.taskIDForTopology(w, r)
	if !ok {
		return
	}
	pg := s.m.pg
	var groups []topoGroup
	rows, _ := pg.Query(`SELECT id,name,color FROM task_topology_groups WHERE task_id=$1 ORDER BY id`, tid)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var g topoGroup
			_ = rows.Scan(&g.ID, &g.Name, &g.Color)
			groups = append(groups, g)
		}
	}
	var nodes []topoNode
	nrows, _ := pg.Query(`SELECT ip,hostname,display_label,group_id,x,y,notes FROM task_topology_nodes WHERE task_id=$1 ORDER BY ip`, tid)
	if nrows != nil {
		defer nrows.Close()
		for nrows.Next() {
			var n topoNode
			_ = nrows.Scan(&n.IP, &n.Hostname, &n.Label, &n.GroupID, &n.X, &n.Y, &n.Notes)
			nodes = append(nodes, n)
		}
	}
	var edges []topoEdge
	erows, _ := pg.Query(`SELECT id,src_ip,dst_ip,label FROM task_topology_edges WHERE task_id=$1 ORDER BY id`, tid)
	if erows != nil {
		defer erows.Close()
		for erows.Next() {
			var e topoEdge
			_ = erows.Scan(&e.ID, &e.Src, &e.Dst, &e.Label)
			edges = append(edges, e)
		}
	}
	var notes []topoNote
	trows, _ := pg.Query(`SELECT id,content,x,y FROM task_topology_notes WHERE task_id=$1 ORDER BY id`, tid)
	if trows != nil {
		defer trows.Close()
		for trows.Next() {
			var n topoNote
			_ = trows.Scan(&n.ID, &n.Content, &n.X, &n.Y)
			notes = append(notes, n)
		}
	}
	writeJSON(w, 200, map[string]any{"nodes": nodes, "edges": edges, "groups": groups, "notes": notes})
}

func (s *Server) upsertTopologyNode(w http.ResponseWriter, r *http.Request) {
	tid, ok := s.taskIDForTopology(w, r)
	if !ok {
		return
	}
	var n topoNode
	if !decodeJSON(w, r, &n, maxSmallJSONBodyBytes) {
		return
	}
	if strings.TrimSpace(n.IP) == "" {
		writeErr(w, 400, "ip 必填")
		return
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
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) deleteTopologyNode(w http.ResponseWriter, r *http.Request) {
	tid, ok := s.taskIDForTopology(w, r)
	if !ok {
		return
	}
	ip := r.PathValue("ip")
	_, _ = s.m.pg.Exec(`DELETE FROM task_topology_edges WHERE task_id=$1 AND (src_ip=$2 OR dst_ip=$2)`, tid, ip)
	_, _ = s.m.pg.Exec(`DELETE FROM task_topology_nodes WHERE task_id=$1 AND ip=$2`, tid, ip)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) addTopologyEdge(w http.ResponseWriter, r *http.Request) {
	tid, ok := s.taskIDForTopology(w, r)
	if !ok {
		return
	}
	var e topoEdge
	if !decodeJSON(w, r, &e, maxSmallJSONBodyBytes) {
		return
	}
	if strings.TrimSpace(e.Src) == "" || strings.TrimSpace(e.Dst) == "" {
		writeErr(w, 400, "src_ip / dst_ip 必填")
		return
	}
	if err := s.m.pg.QueryRow(`INSERT INTO task_topology_edges(task_id,src_ip,dst_ip,label)
		VALUES ($1,$2,$3,$4) RETURNING id`, tid, e.Src, e.Dst, e.Label).Scan(&e.ID); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, e)
}

func (s *Server) patchTopologyEdge(w http.ResponseWriter, r *http.Request) {
	tid, ok := s.taskIDForTopology(w, r)
	if !ok {
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("eid"), 10, 64)
	var e topoEdge
	if !decodeJSON(w, r, &e, maxSmallJSONBodyBytes) {
		return
	}
	_, err := s.m.pg.Exec(`UPDATE task_topology_edges SET label=$3 WHERE task_id=$1 AND id=$2`, tid, id, e.Label)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) deleteTopologyEdge(w http.ResponseWriter, r *http.Request) {
	tid, ok := s.taskIDForTopology(w, r)
	if !ok {
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("eid"), 10, 64)
	_, _ = s.m.pg.Exec(`DELETE FROM task_topology_edges WHERE task_id=$1 AND id=$2`, tid, id)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) upsertTopologyGroup(w http.ResponseWriter, r *http.Request) {
	tid, ok := s.taskIDForTopology(w, r)
	if !ok {
		return
	}
	var g topoGroup
	if !decodeJSON(w, r, &g, maxSmallJSONBodyBytes) {
		return
	}
	if g.ID > 0 {
		_, err := s.m.pg.Exec(`UPDATE task_topology_groups SET name=$3,color=$4 WHERE task_id=$1 AND id=$2`, tid, g.ID, g.Name, g.Color)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	} else {
		if err := s.m.pg.QueryRow(`INSERT INTO task_topology_groups(task_id,name,color) VALUES ($1,$2,$3) RETURNING id`, tid, g.Name, g.Color).Scan(&g.ID); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	writeJSON(w, 200, g)
}

func (s *Server) deleteTopologyGroup(w http.ResponseWriter, r *http.Request) {
	tid, ok := s.taskIDForTopology(w, r)
	if !ok {
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("gid"), 10, 64)
	_, _ = s.m.pg.Exec(`UPDATE task_topology_nodes SET group_id=NULL WHERE task_id=$1 AND group_id=$2`, tid, id)
	_, _ = s.m.pg.Exec(`DELETE FROM task_topology_groups WHERE task_id=$1 AND id=$2`, tid, id)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) addTopologyNote(w http.ResponseWriter, r *http.Request) {
	tid, ok := s.taskIDForTopology(w, r)
	if !ok {
		return
	}
	var n topoNote
	if !decodeJSON(w, r, &n, maxSmallJSONBodyBytes) {
		return
	}
	if err := s.m.pg.QueryRow(`INSERT INTO task_topology_notes(task_id,content,x,y) VALUES ($1,$2,$3,$4) RETURNING id`,
		tid, n.Content, n.X, n.Y).Scan(&n.ID); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, n)
}

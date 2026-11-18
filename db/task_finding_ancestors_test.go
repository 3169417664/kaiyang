package db

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// ancestorFixture builds a source exploration with:
//
//	origin ─derived_from→ A(intent/done) ─yields→ F1(fact) ─derived_from→ B(intent/done) ─yields→ X(finding, dispatched)
//	                 └─yields→ Y(finding, sibling)
//	                                             └─yields→ F2(fact, downstream)
//	origin ─derived_from→ Z(intent, unrelated branch)
//
// X is the triggering finding: only A, F1 and B are its ancestors; Y, F2 and Z
// are unrelated branches and must not be copied. The source origin fact must not
// be copied either.
type ancestorFixture struct {
	sourceTask    *Task
	expID         int64
	originID      int64
	intentA       int64
	factF1        int64
	intentB       int64
	findingX      int64 // exploration node
	findingXID    int64 // findings.id
	assetID       int64
	assetKey      string
	siblingNodes  []int64 // Y, F2, Z
	sourceNodes   int
	sourceEdgeCnt int
}

func buildAncestorFixture(t *testing.T, d *DB) *ancestorFixture {
	t.Helper()
	stamp := time.Now().UnixNano()
	domain := fmt.Sprintf("ancestor-%d.test", stamp)
	assetKey := fmt.Sprintf("ancestor-%d.test", stamp)

	task, err := d.CreateTask("ancestor source", "发现漏洞", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	var assetID int64
	if err := d.QueryRow(`INSERT INTO assets(type, domain, root_domain) VALUES ('service',$1,$1) RETURNING id`, domain).Scan(&assetID); err != nil {
		d.DeleteTask(task.ID)
		t.Fatal(err)
	}

	es := d.Exploration(task.ExplorationID)
	origin, err := es.OriginFactID()
	if err != nil || origin == 0 {
		d.Exec(`DELETE FROM assets WHERE id=$1`, assetID)
		d.DeleteTask(task.ID)
		t.Fatalf("origin fact: id=%d err=%v", origin, err)
	}

	intentA, err := es.AddNode(KindIntent, map[string]any{"summary": "recon internal host"}, 5, "done", "planner", []int64{assetID})
	if err != nil {
		t.Fatal(err)
	}
	if err := es.Link(origin, RelDerivedFrom, intentA); err != nil {
		t.Fatal(err)
	}
	factF1, err := es.AddNode(KindFact, map[string]any{"summary": "service reachable"}, 5, "confirmed", "worker", []int64{assetID})
	if err != nil {
		t.Fatal(err)
	}
	if err := es.Link(intentA, RelYields, factF1); err != nil {
		t.Fatal(err)
	}
	intentB, err := es.AddNode(KindIntent, map[string]any{"summary": "test sql injection"}, 6, "done", "planner", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := es.Link(factF1, RelDerivedFrom, intentB); err != nil {
		t.Fatal(err)
	}
	findingX, err := es.AddNode(KindFinding, map[string]any{"summary": "sql injection confirmed", "severity": "high"}, 8, "confirmed", "worker", []int64{assetID})
	if err != nil {
		t.Fatal(err)
	}
	if err := es.Link(intentB, RelYields, findingX); err != nil {
		t.Fatal(err)
	}
	findingXID, err := d.AddFinding(task.ID, findingX, "sqli", "SQL 注入", "high", "confirmed", "poc", "worker", []int64{assetID})
	if err != nil {
		t.Fatal(err)
	}

	// Unrelated branches.
	siblingFindingY, err := es.AddNode(KindFinding, map[string]any{"summary": "sibling finding"}, 4, "confirmed", "worker", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := es.Link(intentA, RelYields, siblingFindingY); err != nil {
		t.Fatal(err)
	}
	factF2, err := es.AddNode(KindFact, map[string]any{"summary": "downstream fact"}, 4, "confirmed", "worker", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := es.Link(intentB, RelYields, factF2); err != nil {
		t.Fatal(err)
	}
	intentZ, err := es.AddNode(KindIntent, map[string]any{"summary": "unrelated branch"}, 3, "done", "planner", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := es.Link(origin, RelDerivedFrom, intentZ); err != nil {
		t.Fatal(err)
	}

	sourceNodes, err := countRows(d, `SELECT count(*) FROM exploration_nodes WHERE exploration_id=$1`, task.ExplorationID)
	if err != nil {
		t.Fatal(err)
	}
	sourceEdges, err := countRows(d, `SELECT count(*) FROM exploration_edges WHERE exploration_id=$1`, task.ExplorationID)
	if err != nil {
		t.Fatal(err)
	}

	return &ancestorFixture{
		sourceTask:    task,
		expID:         task.ExplorationID,
		originID:      origin,
		intentA:       intentA,
		factF1:        factF1,
		intentB:       intentB,
		findingX:      findingX,
		findingXID:    findingXID,
		assetID:       assetID,
		assetKey:      assetKey,
		siblingNodes:  []int64{siblingFindingY, factF2, intentZ},
		sourceNodes:   sourceNodes,
		sourceEdgeCnt: sourceEdges,
	}
}

func (f *ancestorFixture) cleanup(t *testing.T, d *DB) {
	t.Helper()
	_, _ = d.Exec(`DELETE FROM findings WHERE id=$1`, f.findingXID)
	_ = d.DeleteTask(f.sourceTask.ID)
	_, _ = d.Exec(`DELETE FROM assets WHERE id=$1`, f.assetID)
}

func countRows(d *DB, query string, args ...any) (int, error) {
	var n int
	err := d.QueryRow(query, args...).Scan(&n)
	return n, err
}

// TestCreateTaskWithDispatchFindingCopiesAncestors verifies the spawn_task
// finding_id path copies the ancestor subgraph into the child task's exploration,
// remaps ids/edges/anchors, pins copied intents to done, skips the triggering
// finding and unrelated branches, and is idempotent.
func TestCreateTaskWithDispatchFindingCopiesAncestors(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("postgres unavailable (%v) — skipping", err)
	}
	defer d.Close()

	fx := buildAncestorFixture(t, d)
	defer fx.cleanup(t, d)

	child, err := d.CreateTaskWithOptions("internal follow-up", "利用 SQL 注入", TaskCreateOptions{
		DispatchFindingID: fx.findingXID,
		DispatchAssetKeys: []string{fx.assetKey},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer d.DeleteTask(child.ID)

	childOrigin, err := d.Exploration(child.ExplorationID).OriginFactID()
	if err != nil || childOrigin == 0 {
		t.Fatalf("child origin fact: id=%d err=%v", childOrigin, err)
	}

	// Imported source-node id → child-node id.
	imported := map[int64]int64{}
	rows, err := d.Query(`SELECT payload->>'imported_from_node_id', id, kind, state, payload::text
		FROM exploration_nodes WHERE exploration_id=$1 AND id<>$2`, child.ExplorationID, childOrigin)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var marker, kind, state, payloadText string
		var dstID int64
		if err := rows.Scan(&marker, &dstID, &kind, &state, &payloadText); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		var srcID int64
		if _, err := fmt.Sscanf(marker, "%d", &srcID); err != nil {
			rows.Close()
			t.Fatalf("bad imported marker %q: %v", marker, err)
		}
		imported[srcID] = dstID
		switch kind {
		case KindIntent:
			if state != "done" {
				rows.Close()
				t.Fatalf("imported intent %d state=%q, want done", dstID, state)
			}
		case KindFact:
			if state != "confirmed" {
				rows.Close()
				t.Fatalf("imported fact %d state=%q, want confirmed", dstID, state)
			}
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(payloadText), &payload); err != nil {
			rows.Close()
			t.Fatalf("imported payload is not json: %v", err)
		}
		if got := int64(payload["imported_from_finding_id"].(float64)); got != fx.findingXID {
			rows.Close()
			t.Fatalf("imported node %d finding marker=%d, want %d", dstID, got, fx.findingXID)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	if len(imported) != 3 {
		t.Fatalf("imported node count=%d, want 3 (A/F1/B): %+v", len(imported), imported)
	}
	for _, src := range []int64{fx.intentA, fx.factF1, fx.intentB} {
		if imported[src] == 0 {
			t.Fatalf("ancestor %d was not imported: %+v", src, imported)
		}
	}
	for _, src := range append([]int64{fx.findingX}, fx.siblingNodes...) {
		if imported[src] != 0 {
			t.Fatalf("node %d must not be copied (parent finding / unrelated branch)", src)
		}
	}

	// Edges are remapped onto the copies; the triggering finding's yields edge is gone.
	type edge struct {
		from int64
		rel  string
		to   int64
	}
	var edges []edge
	erows, err := d.Query(`SELECT src_id, rel, dst_id FROM exploration_edges WHERE exploration_id=$1`, child.ExplorationID)
	if err != nil {
		t.Fatal(err)
	}
	for erows.Next() {
		var e edge
		if err := erows.Scan(&e.from, &e.rel, &e.to); err != nil {
			erows.Close()
			t.Fatal(err)
		}
		edges = append(edges, e)
	}
	erows.Close()
	if err := erows.Err(); err != nil {
		t.Fatal(err)
	}
	hasEdge := func(from int64, rel string, to int64) bool {
		for _, e := range edges {
			if e.from == from && e.rel == rel && e.to == to {
				return true
			}
		}
		return false
	}
	if !hasEdge(imported[fx.intentA], RelYields, imported[fx.factF1]) {
		t.Fatalf("missing copied A-yields->F1 edge: %+v", edges)
	}
	if !hasEdge(imported[fx.factF1], RelDerivedFrom, imported[fx.intentB]) {
		t.Fatalf("missing copied F1-derived_from->B edge: %+v", edges)
	}
	if !hasEdge(childOrigin, RelDerivedFrom, imported[fx.intentA]) {
		t.Fatalf("ancestor root A is not anchored to the child origin: %+v", edges)
	}
	for _, e := range edges {
		if e.to == imported[fx.findingX] || e.from == imported[fx.findingX] {
			t.Fatalf("triggering finding id leaked into an edge: %+v", e)
		}
	}

	// Asset anchors are carried over to the remapped node ids.
	var anchors int
	if err := d.QueryRow(`SELECT count(*) FROM exploration_anchors WHERE node_id=$1 AND asset_id=$2`,
		imported[fx.factF1], fx.assetID).Scan(&anchors); err != nil {
		t.Fatal(err)
	}
	if anchors != 1 {
		t.Fatalf("fact F1 anchors=%d, want 1", anchors)
	}

	// The source exploration is untouched.
	if n, err := countRows(d, `SELECT count(*) FROM exploration_nodes WHERE exploration_id=$1`, fx.expID); err != nil || n != fx.sourceNodes {
		t.Fatalf("source nodes changed: got %d want %d (err=%v)", n, fx.sourceNodes, err)
	}

	// Idempotent: running the copy again on the same destination changes nothing.
	nodesBefore, err := countRows(d, `SELECT count(*) FROM exploration_nodes WHERE exploration_id=$1`, child.ExplorationID)
	if err != nil {
		t.Fatal(err)
	}
	edgesBefore := len(edges)
	tx, err := d.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := copyFindingAncestorsTx(tx, fx.findingXID, child.ExplorationID, childOrigin); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	nodesAfter, err := countRows(d, `SELECT count(*) FROM exploration_nodes WHERE exploration_id=$1`, child.ExplorationID)
	if err != nil {
		t.Fatal(err)
	}
	edgesAfter, err := countRows(d, `SELECT count(*) FROM exploration_edges WHERE exploration_id=$1`, child.ExplorationID)
	if err != nil {
		t.Fatal(err)
	}
	if nodesAfter != nodesBefore || edgesAfter != edgesBefore {
		t.Fatalf("repeat copy is not idempotent: nodes %d->%d edges %d->%d", nodesBefore, nodesAfter, edgesBefore, edgesAfter)
	}
}

// TestCreateTaskWithoutDispatchFindingKeepsPlainGraph is the regression guard:
// a normal spawn_task without finding_id must still create exactly the origin
// fact and nothing imported.
func TestCreateTaskWithoutDispatchFindingKeepsPlainGraph(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("postgres unavailable (%v) — skipping", err)
	}
	defer d.Close()

	task, err := d.CreateTaskWithOptions("plain child", "no finding", TaskCreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer d.DeleteTask(task.ID)

	nodes, err := d.Exploration(task.ExplorationID).Nodes(100)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 || nodes[0].Kind != KindFact || nodes[0].State != StateOrigin {
		t.Fatalf("plain task should only have its origin fact, got %+v", nodes)
	}
	var imported int
	if err := d.QueryRow(`SELECT count(*) FROM exploration_nodes
		WHERE exploration_id=$1 AND payload ? $2`, task.ExplorationID, importedFromNodeField).Scan(&imported); err != nil {
		t.Fatal(err)
	}
	if imported != 0 {
		t.Fatalf("plain task has %d imported nodes, want 0", imported)
	}
}

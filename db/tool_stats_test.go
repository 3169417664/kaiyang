package db

import "testing"

func TestToolOutcomeStats(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	defer d.Close()
	expID, err := d.CreateExploration("tool stats fixture", "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = d.Exec(`DELETE FROM explorations WHERE id=$1`, expID) })
	es := d.Exploration(expID)

	intent, err := es.AddNode(KindIntent, map[string]any{"summary": "recon"}, 0, "done", "worker", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := es.RecordIntentReflection(IntentReflection{
		IntentID: intent, Dimension: "recon", TerminalState: "done",
		Summary: "recon", Lesson: "lesson",
	}); err != nil {
		t.Fatal(err)
	}
	insert := func(tool, kind string, isErr bool) {
		if _, err := d.Exec(`INSERT INTO activity(exploration_id,node_id,worker,kind,tool,is_error)
VALUES ($1,$2,'worker',$3,$4,$5)`, expID, intent, kind, tool, isErr); err != nil {
			t.Fatal(err)
		}
	}
	// Bash 10 次调用 / 2 次错误；http_request 5 次 / 0 错误。
	for i := 0; i < 10; i++ {
		insert("Bash", "tool_use", false)
	}
	for i := 0; i < 2; i++ {
		insert("Bash", "tool_result", true)
	}
	for i := 0; i < 5; i++ {
		insert("http_request", "tool_use", false)
	}
	// 样本不足的工具（2 次）不应出现。
	for i := 0; i < 2; i++ {
		insert("nmap", "tool_use", false)
	}

	stats, err := es.ToolOutcomeStats("recon", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 2 {
		t.Fatalf("stats = %+v, want 2 tools", stats)
	}
	if stats[0].Tool != "http_request" || stats[0].Uses != 5 || stats[0].Errors != 0 {
		t.Fatalf("best tool = %+v", stats[0])
	}
	if stats[1].Tool != "Bash" || stats[1].Uses != 10 || stats[1].Errors != 2 {
		t.Fatalf("second tool = %+v", stats[1])
	}
	// 其他维度无数据。
	if other, err := es.ToolOutcomeStats("authz", 5); err != nil || len(other) != 0 {
		t.Fatalf("other dimension = %+v, %v", other, err)
	}
}

func TestHighErrorTools(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	defer d.Close()
	expID, err := d.CreateExploration("high error fixture", "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = d.Exec(`DELETE FROM explorations WHERE id=$1`, expID) })
	es := d.Exploration(expID)

	intent, err := es.AddNode(KindIntent, map[string]any{"summary": "recon"}, 0, "done", "worker", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := es.RecordIntentReflection(IntentReflection{
		IntentID: intent, Dimension: "recon", TerminalState: "done",
	}); err != nil {
		t.Fatal(err)
	}
	insert := func(tool, kind string, isErr bool) {
		if _, err := d.Exec(`INSERT INTO activity(exploration_id,node_id,worker,kind,tool,is_error)
VALUES ($1,$2,'worker',$3,$4,$5)`, expID, intent, kind, tool, isErr); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 10; i++ {
		insert("nmap", "tool_use", false)
	}
	for i := 0; i < 7; i++ {
		insert("nmap", "tool_result", true)
	}
	for i := 0; i < 10; i++ {
		insert("Bash", "tool_use", false)
	}
	// 错误率 20%，低于阈值。
	for i := 0; i < 2; i++ {
		insert("Bash", "tool_result", true)
	}

	risky, err := es.HighErrorTools("recon", 5, 0.6, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(risky) != 1 || risky[0].Tool != "nmap" || risky[0].Uses != 10 || risky[0].Errors != 7 {
		t.Fatalf("risky = %+v", risky)
	}
}

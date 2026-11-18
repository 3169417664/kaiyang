package server

import "testing"

func TestParseActivitySessionAll(t *testing.T) {
	f, ok := parseActivitySession("all")
	if !ok {
		t.Fatal("session=all 应被接受")
	}
	if f.Main || f.Worker != "" || f.NodeID != nil || f.MainSeg != nil {
		t.Fatalf("session=all 应返回零值过滤（全部 lane），得到 %+v", f)
	}
}

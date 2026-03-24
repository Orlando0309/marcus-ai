package task

import "testing"

func TestDAGTopologicalOrder(t *testing.T) {
	d := &DAG{
		Nodes: []string{"a", "b", "c"},
		Edges: []Edge{{From: "a", To: "b"}, {From: "b", To: "c"}},
	}
	ord, err := d.TopologicalOrder()
	if err != nil {
		t.Fatal(err)
	}
	pos := map[string]int{}
	for i, id := range ord {
		pos[id] = i
	}
	if pos["a"] > pos["b"] || pos["b"] > pos["c"] {
		t.Fatalf("bad order: %v", ord)
	}
}

func TestDAGCycle(t *testing.T) {
	d := &DAG{
		Edges: []Edge{{From: "a", To: "b"}, {From: "b", To: "a"}},
	}
	if _, err := d.TopologicalOrder(); err == nil {
		t.Fatal("expected cycle error")
	}
}

func TestDAGReady(t *testing.T) {
	d := &DAG{Edges: []Edge{{From: "a", To: "b"}}}
	done := map[string]bool{"a": true}
	r := d.Ready(done)
	if len(r) != 1 || r[0] != "b" {
		t.Fatalf("got %v", r)
	}
}

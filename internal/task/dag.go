package task

import (
	"fmt"
	"sort"
	"strings"
)

// Edge is a dependency edge From -> To for lightweight workflow graphs.
// From must complete before To can start.
type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// DAG holds task ids and directed edges (orchestration extension point).
type DAG struct {
	Nodes []string `json:"nodes"`
	Edges []Edge   `json:"edges"`
}

func nodeNames(d *DAG) map[string]struct{} {
	m := map[string]struct{}{}
	for _, n := range d.Nodes {
		n = strings.TrimSpace(n)
		if n != "" {
			m[n] = struct{}{}
		}
	}
	for _, e := range d.Edges {
		if strings.TrimSpace(e.From) != "" {
			m[e.From] = struct{}{}
		}
		if strings.TrimSpace(e.To) != "" {
			m[e.To] = struct{}{}
		}
	}
	return m
}

// TopologicalOrder returns nodes in dependency order (Kahn). Fails on cycles.
func (d *DAG) TopologicalOrder() ([]string, error) {
	if d == nil {
		return nil, nil
	}
	nodes := nodeNames(d)
	if len(nodes) == 0 {
		return nil, nil
	}
	inDeg := make(map[string]int, len(nodes))
	adj := make(map[string][]string)
	for n := range nodes {
		inDeg[n] = 0
	}
	for _, e := range d.Edges {
		from, to := strings.TrimSpace(e.From), strings.TrimSpace(e.To)
		if from == "" || to == "" {
			continue
		}
		adj[from] = append(adj[from], to)
		inDeg[to]++
	}
	var q []string
	for n := range nodes {
		if inDeg[n] == 0 {
			q = append(q, n)
		}
	}
	sort.Strings(q)
	var out []string
	for len(q) > 0 {
		u := q[0]
		q = q[1:]
		out = append(out, u)
		succ := append([]string(nil), adj[u]...)
		sort.Strings(succ)
		for _, v := range succ {
			inDeg[v]--
			if inDeg[v] == 0 {
				q = append(q, v)
				sort.Strings(q)
			}
		}
	}
	if len(out) != len(nodes) {
		return nil, fmt.Errorf("workflow DAG has a cycle")
	}
	return out, nil
}

// Ready returns task IDs that are not done and have all predecessors done.
func (d *DAG) Ready(done map[string]bool) []string {
	if d == nil {
		return nil
	}
	if done == nil {
		done = map[string]bool{}
	}
	nodes := nodeNames(d)
	var out []string
	for n := range nodes {
		if done[n] {
			continue
		}
		blocked := false
		for _, e := range d.Edges {
			if strings.TrimSpace(e.To) != n || strings.TrimSpace(e.From) == "" {
				continue
			}
			if !done[e.From] {
				blocked = true
				break
			}
		}
		if !blocked {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

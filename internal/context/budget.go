package context

import (
	"sort"
	"strings"
)

type contextBlock struct {
	name   string
	text   string
	keep   int // higher = retain under pressure (dropped lowest first)
	order  int // final sort order (ascending)
}

func applyTokenBudget(blocks []contextBlock, maxTokens int) (string, int, bool, []string) {
	if maxTokens <= 0 {
		var b strings.Builder
		var total int
		for _, bl := range sortedByOrder(blocks) {
			b.WriteString(bl.text)
			if !strings.HasSuffix(bl.text, "\n") {
				b.WriteByte('\n')
			}
			b.WriteByte('\n')
			total += EstimateTokens(bl.text)
		}
		return strings.TrimSpace(b.String()), total, false, nil
	}

	// Work on a copy sorted by keep (ascending) for dropping
	byKeep := append([]contextBlock(nil), blocks...)
	sort.Slice(byKeep, func(i, j int) bool {
		if byKeep[i].keep == byKeep[j].keep {
			return byKeep[i].order > byKeep[j].order // drop later sections first at same keep
		}
		return byKeep[i].keep < byKeep[j].keep
	})

	kept := append([]contextBlock(nil), blocks...)
	dropped := map[string]bool{}

	for {
		total := 0
		for _, bl := range kept {
			total += EstimateTokens(bl.text)
		}
		if total <= maxTokens {
			break
		}
		// Drop lowest-keep block not yet dropped
		var victim *contextBlock
		for i := range byKeep {
			if dropped[byKeep[i].name] {
				continue
			}
			victim = &byKeep[i]
			break
		}
		if victim == nil {
			break
		}
		dropped[victim.name] = true
		out := kept[:0]
		for _, bl := range kept {
			if !dropped[bl.name] {
				out = append(out, bl)
			}
		}
		kept = out
	}

	var names []string
	for n := range dropped {
		names = append(names, n)
	}
	sort.Strings(names)

	var b strings.Builder
	total := 0
	for _, bl := range sortedByOrder(kept) {
		b.WriteString(bl.text)
		if !strings.HasSuffix(bl.text, "\n") {
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
		total += EstimateTokens(bl.text)
	}
	truncated := len(dropped) > 0
	return strings.TrimSpace(b.String()), total, truncated, names
}

func sortedByOrder(blocks []contextBlock) []contextBlock {
	out := append([]contextBlock(nil), blocks...)
	sort.Slice(out, func(i, j int) bool { return out[i].order < out[j].order })
	return out
}

func fileRelevanceScore(input, relPath, content string) int {
	input = strings.ToLower(input)
	relPath = strings.ToLower(relPath)
	score := 10
	for _, tok := range tokenizeForScore(input) {
		if len(tok) < 3 {
			continue
		}
		if strings.Contains(relPath, tok) {
			score += 5
		}
		if strings.Contains(strings.ToLower(content), tok) {
			score += 2
		}
	}
	return score
}

func tokenizeForScore(s string) []string {
	replacer := strings.NewReplacer("\n", " ", "\t", " ", ",", " ", ";", " ")
	s = replacer.Replace(s)
	var out []string
	for _, w := range strings.Fields(s) {
		w = strings.Trim(w, "@./\\\":()[]{}'")
		if w != "" {
			out = append(out, w)
		}
	}
	return out
}

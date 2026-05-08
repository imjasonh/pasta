package engine

import (
	"sort"

	"github.com/imjasonh/pasta/internal/dsl"
	"github.com/imjasonh/pasta/internal/lang"
)

// ruleGroup is one strongly-connected component of the rule dependency
// graph. Single-rule groups with no self-loop run exactly once;
// multi-rule groups OR single-rule groups whose Provides intersects
// Requires run as a fixpoint until the fact store stops growing.
type ruleGroup struct {
	rules    []scheduledRule
	fixpoint bool
}

// scheduleGroups collects every rule applicable to language l, builds
// the dependency graph (provider rules -> consumer rules), finds SCCs
// via Tarjan's algorithm, and returns the SCCs in topological order.
//
// SCCs of size 1 with no self-loop run once. SCCs of size 1 whose
// Provides intersects Requires (or any SCC of size > 1) run as a
// fixpoint group until convergence.
func scheduleGroups(analyzers []*dsl.Analyzer, l lang.Language) ([]ruleGroup, error) {
	var all []scheduledRule
	for _, a := range analyzers {
		for name, rule := range a.Rules {
			if !ruleAppliesToLanguage(&rule, l) {
				continue
			}
			all = append(all, scheduledRule{analyzer: a, name: name, rule: rule})
		}
	}
	// Deterministic ordering before building the graph.
	sort.Slice(all, func(i, j int) bool {
		if all[i].analyzer.Name != all[j].analyzer.Name {
			return all[i].analyzer.Name < all[j].analyzer.Name
		}
		return all[i].name < all[j].name
	})

	providers := map[string][]int{}
	for i, r := range all {
		for _, f := range r.rule.Provides {
			providers[f] = append(providers[f], i)
		}
	}
	g := make([][]int, len(all))
	for i, r := range all {
		for _, req := range r.rule.Requires {
			for _, j := range providers[req] {
				g[j] = append(g[j], i)
			}
		}
	}

	// Tarjan's algorithm. Returns SCCs in reverse topological order.
	sccs := tarjanSCCs(g)

	// Reverse to get topo order.
	groups := make([]ruleGroup, 0, len(sccs))
	for i := len(sccs) - 1; i >= 0; i-- {
		scc := sccs[i]
		// Sort within the SCC for deterministic iteration order.
		sort.Slice(scc, func(a, b int) bool {
			return all[scc[a]].name < all[scc[b]].name
		})
		rules := make([]scheduledRule, len(scc))
		for k, idx := range scc {
			rules[k] = all[idx]
		}

		fixpoint := len(scc) > 1
		if !fixpoint {
			// Single rule: fixpoint only if it has a self-loop (its
			// Provides intersects its Requires).
			r := all[scc[0]]
			for _, p := range r.rule.Provides {
				for _, req := range r.rule.Requires {
					if p == req {
						fixpoint = true
					}
				}
			}
		}
		groups = append(groups, ruleGroup{rules: rules, fixpoint: fixpoint})
	}
	return groups, nil
}

// tarjanSCCs finds strongly-connected components in the directed graph
// g. Returns SCCs in reverse topological order — the first SCC has no
// outgoing edges to a later SCC.
func tarjanSCCs(g [][]int) [][]int {
	n := len(g)
	index := make([]int, n)
	lowlink := make([]int, n)
	onStack := make([]bool, n)
	for i := range index {
		index[i] = -1
	}
	var stack []int
	counter := 0
	var sccs [][]int

	var strongconnect func(v int)
	strongconnect = func(v int) {
		index[v] = counter
		lowlink[v] = counter
		counter++
		stack = append(stack, v)
		onStack[v] = true

		for _, w := range g[v] {
			if index[w] == -1 {
				strongconnect(w)
				if lowlink[w] < lowlink[v] {
					lowlink[v] = lowlink[w]
				}
			} else if onStack[w] {
				if index[w] < lowlink[v] {
					lowlink[v] = index[w]
				}
			}
		}

		if lowlink[v] == index[v] {
			var scc []int
			for {
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				onStack[w] = false
				scc = append(scc, w)
				if w == v {
					break
				}
			}
			sccs = append(sccs, scc)
		}
	}

	for v := 0; v < n; v++ {
		if index[v] == -1 {
			strongconnect(v)
		}
	}
	return sccs
}

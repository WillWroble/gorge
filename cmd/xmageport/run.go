package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/rules"
)

// materialize returns a scenario copy with every answer group placed at its
// current step (base + offset).
func materialize(sc *portScenario, groups []*group) *portScenario {
	cp := *sc
	cp.Steps = make([]*step, len(sc.Steps))
	for i, s := range sc.Steps {
		c := *s
		c.Targets = append([]string(nil), s.Targets...)
		c.Answers = append([]answer(nil), s.Answers...)
		cp.Steps[i] = &c
	}
	for _, g := range groups {
		i := g.at + g.offset
		if i >= len(cp.Steps) {
			i = len(cp.Steps) - 1
		}
		cp.Steps[i].Targets = append(cp.Steps[i].Targets, g.targets...)
		cp.Steps[i].Answers = append(cp.Steps[i].Answers, g.placed()...)
	}
	return &cp
}

var reStepFail = regexp.MustCompile(`^(?:after )?step (\d+) \(`)

// placementFailSteps returns the step indices whose failure is about an
// answer or target the step did not consume.
func placementFailSteps(fails []string) map[int]bool {
	out := map[int]bool{}
	for _, f := range fails {
		m := reStepFail.FindStringSubmatch(f)
		if m == nil {
			continue
		}
		if strings.Contains(f, "unconsumed answer(s)") || strings.Contains(f, "unused target(s)") ||
			(strings.Contains(f, "harness: answer ") && strings.Contains(f, "not offered")) ||
			(strings.Contains(f, "harness: target ") && strings.Contains(f, "not offered")) {
			n, _ := strconv.Atoi(m[1])
			out[n] = true
		}
	}
	return out
}

var verbose bool

func runOne(reg *cards.Registry, sc *portScenario) ([]string, error) {
	raw, err := json.Marshal(scenarioBody(sc))
	if err != nil {
		return nil, err
	}
	res, err := rules.RunOracleScenarioJSON(reg, raw)
	if err != nil {
		return nil, err
	}
	if verbose {
		fmt.Println(string(raw))
		fmt.Println(strings.Join(res.Transcript, "\n"))
		fmt.Println(strings.Join(res.Fails, "\n"))
	}
	return res.Fails, nil
}

// scenarioBody drops the wrapper fields rules' strict decoder does not know.
func scenarioBody(sc *portScenario) any {
	type body struct {
		Name   string           `json:"name"`
		Turn   *int             `json:"turn,omitempty"`
		CR     []string         `json:"cr"`
		Why    string           `json:"why"`
		Setup  map[string]*seat `json:"setup"`
		Steps  []*step          `json:"steps"`
		Expect []*expect        `json:"expect"`
	}
	return body{sc.Name, sc.Turn, sc.CR, sc.Why, sc.Setup, sc.Steps, sc.Expect}
}

var reNotOffered = regexp.MustCompile(`harness: answer "((?:[^"\\]|\\.)*)" not offered for (\w+):`)

// answerKinds lists the decision kinds an XMage choice may be answering, in
// the order tried: "any" first, then the specific kinds, so a choice the
// wildcard hands to the wrong decision (an attackers ask) is retried on
// the kinds a setChoice can mean.
func answerKinds(a answer) []string {
	if a.Kind != "any" {
		return []string{a.Kind}
	}
	p := strings.ToLower(a.Pick[0])
	switch {
	case p == "yes" || p == "no":
		return []string{"any", "trigger_optional", "choose", "replacement", "modes"}
	case strings.HasPrefix(p, "p0:") || strings.HasPrefix(p, "p1:") || p == "p0" || p == "p1":
		return []string{"any", "choose", "target", "replacement", "arrange", "modes"}
	}
	return []string{"any", "choose", "modes", "replacement", "trigger_order"}
}

func (g *group) placed() []answer {
	out := make([]answer, len(g.answers))
	for i, a := range g.answers {
		ks := answerKinds(a)
		k := 0
		if i < len(g.kinds) && g.kinds[i] < len(ks) {
			k = g.kinds[i]
		}
		out[i] = answer{Kind: ks[k], Pick: a.Pick, Seat: a.Seat}
	}
	return out
}

// takeLeftovers removes from g the answers/targets a leftover failure lists
// and returns them as a new group (nil when none matched). A choice that
// reaches the last step unconsumed is dropped: gorge never poses it, and the
// expectations still come from XMage, so dropping it cannot make a wrong
// game pass -- it only lets the asserts judge the game.
func takeLeftovers(g *group, line string) *group {
	i := strings.Index(line, "[")
	if i < 0 {
		return nil
	}
	out := &group{}
	if strings.Contains(line, "unconsumed answer(s)") {
		var left []answer
		if json.Unmarshal([]byte(line[i:]), &left) != nil {
			return nil
		}
		for _, l := range left {
			for j, a := range g.answers {
				if len(a.Pick) > 0 && len(l.Pick) > 0 && a.Pick[0] == l.Pick[0] {
					out.answers = append(out.answers, a)
					out.kinds = append(out.kinds, 0)
					g.answers = append(g.answers[:j:j], g.answers[j+1:]...)
					g.kinds = append(g.kinds[:j:j], g.kinds[j+1:]...)
					break
				}
			}
		}
	} else {
		var left []string
		if json.Unmarshal([]byte(line[i:]), &left) != nil {
			return nil
		}
		for _, l := range left {
			for j, x := range g.targets {
				if x == l {
					out.targets = append(out.targets, x)
					g.targets = append(g.targets[:j:j], g.targets[j+1:]...)
					break
				}
			}
		}
	}
	if len(out.answers)+len(out.targets) == 0 {
		return nil
	}
	return out
}

func runResult(reg *cards.Registry, r *result) {
	sc := r.Scenario
	for _, g := range r.Groups {
		g.kinds = make([]int, len(g.answers))
	}
	var fails []string
	for iter := 0; iter < 40; iter++ {
		cur := materialize(sc, r.Groups)
		f, err := runOne(reg, cur)
		ngroups := len(r.Groups)
		if err != nil {
			fails = []string{"decode: " + err.Error()}
			break
		}
		fails = f
		progress := false
		bump := func(g *group) bool {
			if g.at+g.offset+1 < len(sc.Steps) {
				g.offset++
				for i := range g.kinds {
					g.kinds[i] = 0
				}
				progress = true
				return true
			}
			return false
		}
		for _, line := range f {
			m := reStepFail.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			n, _ := strconv.Atoi(m[1])
			var at []*group
			for _, g := range r.Groups[:ngroups] {
				if g.at+g.offset == n {
					at = append(at, g)
				}
			}
			if len(at) == 0 {
				continue
			}
			if nm := reNotOffered.FindStringSubmatch(line); nm != nil {
				done := false
				for _, g := range at {
					for i, a := range g.answers {
						if done || a.Pick[0] != nm[1] {
							continue
						}
						ks := answerKinds(a)
						k := g.kinds[i] + 1
						for k < len(ks) && ks[k] == nm[2] {
							k++
						}
						if k < len(ks) {
							g.kinds[i] = k
							progress = true
						} else {
							bump(g)
						}
						done = true
					}
				}
				if !done {
					for _, g := range at {
						bump(g)
					}
				}
				continue
			}
			if strings.Contains(line, "unconsumed answer(s)") || strings.Contains(line, "unused target(s)") {
				// Only the LEFTOVERS move on: what this step consumed stays.
				for _, g := range at {
					left := takeLeftovers(g, line)
					if left == nil {
						continue
					}
					progress = true
					if n+1 < len(sc.Steps) {
						left.at = n + 1
						r.Groups = append(r.Groups, left)
					} else {
						r.Dropped += len(left.answers) + len(left.targets)
					}
				}
				continue
			}
			if strings.Contains(line, "harness: target ") && strings.Contains(line, "not offered") {
				for _, g := range at {
					bump(g)
				}
			}
		}
		if !progress {
			break
		}
	}
	// freeze the placement into the scenario
	final := materialize(sc, r.Groups)
	r.XAnswers = map[int][]xans{}
	for _, g := range r.Groups {
		i := min(g.at+g.offset, len(sc.Steps)-1)
		r.XAnswers[i] = append(r.XAnswers[i], g.xvals...)
	}
	r.Scenario = final
	r.Groups = nil
	r.Fails = fails
	if r.Dropped > 0 {
		r.Scenario.Why += fmt.Sprintf(" %d XMage choice(s) gorge never asks were dropped.", r.Dropped)
	}
	switch {
	case len(fails) == 0:
		r.Status = "pass"
	case anyHarness(fails):
		r.Status = "fail-harness"
	default:
		r.Status = "fail-expect"
	}
}

func anyHarness(fails []string) bool {
	for _, f := range fails {
		if strings.Contains(f, "harness:") || strings.Contains(f, "unconsumed answer(s)") || strings.Contains(f, "unused target(s)") || strings.HasPrefix(f, "decode:") {
			return true
		}
	}
	return false
}

func runAll(reg *cards.Registry, results []*result) {
	work := make(chan *result)
	var wg sync.WaitGroup
	n := runtime.GOMAXPROCS(0)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for r := range work {
				func() {
					defer func() {
						if p := recover(); p != nil {
							r.Fails = []string{fmt.Sprintf("harness: panic %v", p)}
							r.Status = "fail-harness"
						}
					}()
					runResult(reg, r)
				}()
			}
		}()
	}
	for _, r := range results {
		if r.Scenario != nil {
			work <- r
		}
	}
	close(work)
	wg.Wait()
}

// driverLine renders r's scenario as an input line for the XMage oracle
// driver (scripts/xmage-oracle-run.sh): gorge-only `pass` steps and gorge
// answer placements are removed, and XMage gets the test's ORIGINAL choices
// (xmage_answers) and activateAbility texts (xmage_ability) instead.
func driverLine(r *result) ([]byte, error) {
	type dstep struct {
		*step
		Answers []answer `json:"answers,omitempty"`
	}
	var steps []any
	var xab []string
	var xan [][]xans
	var carry []xans
	for i, s := range r.Scenario.Steps {
		carry = append(carry, r.XAnswers[i]...)
		if s.Op == "pass" {
			continue
		}
		c := *s
		c.Answers = nil
		steps = append(steps, dstep{step: &c})
		xab = append(xab, s.XAbility)
		xan = append(xan, carry)
		carry = nil
	}
	if len(carry) > 0 && len(xan) > 0 {
		xan[len(xan)-1] = append(xan[len(xan)-1], carry...)
	}
	for i := range xan {
		if xan[i] == nil {
			xan[i] = []xans{}
		}
	}
	sc := r.Scenario
	return json.Marshal(map[string]any{
		"name": r.Class + "#" + r.Method, "cr": sc.CR, "why": sc.Why, "setup": sc.Setup,
		"steps": steps, "expect": sc.Expect, "xmage_ability": xab, "xmage_answers": xan,
	})
}

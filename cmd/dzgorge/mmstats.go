package main

import (
	"fmt"

	"github.com/adams-shaun/gorge/decision"
	"github.com/adams-shaun/gorge/rules"
	"github.com/adams-shaun/gorge/state"
)

// mmStats is the paid-cost mana census A/B probe (play -mm-stats, with or
// without -no-paid-mana-census): per seat, counters over its posed priority
// windows. They are written into each game's record and summed by the report
// script (docs/superpowers/reports/2026-10-12-mana-model-ab.md).
//
// A "new" option is one the census adds: present in the priority walk with
// the paid-cost census on, absent with it off, at the same state. The other
// value's list is derived on a clone (Engine.PriorityOptionsWithPaidManaCensus).
// In a flag-on game new options are those the seat was offered and chose
// from; in a flag-off game they are the exposure (offered under on, not now).
type mmStats map[string]int64

func (m mmStats) add(k string, n int64) { m[k] += n }

// mmOptKey identifies an option across the two walks (Index may shift).
func mmOptKey(o decision.Option) string {
	return fmt.Sprintf("%s|%d|%s|%s|%d", o.Kind, o.Obj, o.Label, o.ManaSymbol, o.Player)
}

// mmTurnBucket groups gorge's turn counter (both players' turns): 1-4 is the
// span :explore samples by visits.
func mmTurnBucket(turn int32) string {
	switch {
	case turn <= 4:
		return "1-4"
	case turn <= 8:
		return "5-8"
	case turn <= 16:
		return "9-16"
	}
	return "17+"
}

func mmBucket(n int) string {
	switch {
	case n == 0:
		return "0"
	case n == 1:
		return "1"
	}
	return "2+"
}

// mmVisibleList is the option list a seat's policy ranks, minus pass, concede
// and mana taps; for an auto-pay seat it adds one entry per planned cast the
// ordinary list lacks (cmd/manamodelstats visibleList).
func mmVisibleList(d *decision.Decision, auto bool) (all, spells int) {
	legacyOrdinary := map[state.ObjID]bool{}
	for _, o := range d.Options {
		switch o.Kind {
		case "pass", "concede", "activate":
			continue
		}
		all++
		if o.Kind != "play_land" {
			spells++
		}
		if o.Kind == "cast" && o.Mode == "" && o.AltCostIndex == 0 {
			legacyOrdinary[o.Obj] = true
		}
	}
	if auto {
		for _, a := range d.PaymentActions {
			if len(a.Plans) == 0 || legacyOrdinary[a.Cast.Object] {
				continue
			}
			all++
			spells++
		}
	}
	return all, spells
}

// observe folds one answered decision into m. engineFlag is the engine's
// paid-cost census setting (the inverse of Config.DisablePaidManaCensus);
// auto is whether the seat pays automatically.
func (m mmStats) observe(e *rules.Engine, engineFlag, auto bool, d *decision.Decision, in decision.Intent) {
	if d.Kind != decision.KPriority {
		return
	}
	m.add("windows", 1)
	m.add("turn:"+mmTurnBucket(e.G.Turn), 1)
	all, spells := mmVisibleList(d, auto)
	own := e.G.Active == d.Player && e.G.Step.IsMain() && len(e.G.Stack) == 0
	m.add("list:all:"+mmBucket(all), 1)
	m.add("list:all:sum", int64(all))
	m.add("list:spells:"+mmBucket(spells), 1)
	m.add("list:spells:sum", int64(spells))
	if own {
		m.add("ownmain:windows", 1)
		m.add("ownmain:list:all:"+mmBucket(all), 1)
		m.add("ownmain:list:all:sum", int64(all))
		m.add("ownmain:list:spells:"+mmBucket(spells), 1)
		m.add("ownmain:list:spells:sum", int64(spells))
	}
	other := e.PriorityOptionsWithPaidManaCensus(!engineFlag)
	// The probe at the engine's own value must reproduce the posed list;
	// count the windows where it does not (a stale memo would show here).
	same := e.PriorityOptionsWithPaidManaCensus(engineFlag)
	if len(same) != len(d.Options) {
		m.add("probe:mismatch", 1)
	} else {
		for i := range same {
			if mmOptKey(same[i]) != mmOptKey(d.Options[i]) {
				m.add("probe:mismatch", 1)
				break
			}
		}
	}
	on, off := d.Options, other
	if !engineFlag {
		on, off = other, d.Options
	}
	offKeys := map[string]bool{}
	for _, o := range off {
		offKeys[mmOptKey(o)] = true
	}
	newKeys := map[string]bool{}
	for _, o := range on {
		if k := mmOptKey(o); !offKeys[k] {
			newKeys[k] = true
			m.add("new:options", 1)
			m.add("new:kind:"+o.Kind, 1)
		}
	}
	if len(newKeys) == 0 {
		return
	}
	m.add("new:windows", 1)
	tb := mmTurnBucket(e.G.Turn)
	m.add("new:turn:"+tb, 1)
	if own {
		m.add("new:ownmain:windows", 1)
	}
	if all <= 1 {
		m.add("new:windows:list<=1", 1)
	}
	if engineFlag && len(in.Choices) == 1 {
		for _, o := range d.Options {
			if o.Index == in.Choices[0] && newKeys[mmOptKey(o)] {
				m.add("new:chosen", 1)
				m.add("new:chosen:turn:"+tb, 1)
				m.add("new:chosen:kind:"+o.Kind, 1)
			}
		}
	}
}

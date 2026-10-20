package rules

import (
	"reflect"
	"sort"

	"github.com/adams-shaun/gorge/engine/decision"
)

// EngineState visits the engine's dynamic state outside state.Game, by field
// name: every clone:"deep" field engineStateSkip does not name, when it holds
// anything (ledgers, trigger counts, continuous effects, last-known
// information, the continuation of a cast or resolution paused at a
// decision), then a posed resolution's tape as "tape". view.ProjectDynamic
// renders it, which is what makes the view a complete game state (principle
// 1 of mz-gorge-design.md). It is a pure read.
func (e *Engine) EngineState(visit func(field string, v reflect.Value)) {
	var walk func(v reflect.Value)
	walk = func(v reflect.Value) {
		t := v.Type()
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.Anonymous {
				walk(v.Field(i))
				continue
			}
			if tag := f.Tag.Get("clone"); len(tag) < 4 || tag[:4] != "deep" || skipsEngineState(f.Name) {
				continue
			}
			if fv := v.Field(i); !fv.IsZero() {
				visit(f.Name, fv)
			}
		}
	}
	walk(reflect.ValueOf(e).Elem())
	// A posed resolution is what the stack and its tape say it is; the
	// checkpoint the kernel holds to re-run it is machinery.
	if tape := e.tape.PosedTape(e.L); tape != nil {
		answers := make([]decision.Intent, len(tape))
		for i, in := range tape {
			in.Seq = 0 // positional, not state
			answers[i] = in
		}
		visit("tape", reflect.ValueOf(answers))
	}
}

func skipsEngineState(field string) bool {
	i := sort.SearchStrings(engineStateSkip, field)
	return i < len(engineStateSkip) && engineStateSkip[i] == field
}

// engineStateSkip names, sorted, the clone:"deep" fields that are not
// dynamic game state:
//   - G: state.Game, which the view projects itself;
//   - L: the event log, which is history;
//   - rng: chance machinery (the state is the game, not the generator);
//   - pending: the pending decision, View.Decision;
//   - tape: visited separately, as the posed resolution's tape;
//   - per-game or corpus-derived configuration: format, mulligans,
//     startingLife, paidManaCensus, windowDiagnostics, layer4InPool,
//     setNameInPool, controlStaticInPool;
//   - diagnostics: AbortTally, LegacyAskTally, phaseUnknownNoted,
//     disableTriggersNoted;
//   - memos and their keys, derived from the game: every other entry.
var engineStateSkip = []string{
	"AbortTally", "G", "L", "LegacyAskTally", "actIndex", "atkOffersActive",
	"atkOffersEp", "atkOffersObjs", "atkOffersVer", "controlStaticInPool",
	"disableTriggersNoted", "format", "layer4InPool", "layer4Types", "lossProof",
	"mulligans", "paidManaCensus", "pending", "phaseUnknownNoted", "renameEpoch",
	"renameObjs", "renameVersion", "renames", "replArena", "replZones",
	"replZonesEp", "rng", "sbaQuiet", "setNameInPool", "startingLife",
	"staticContinuous", "staticEpoch", "staticGates", "staticGatesKnown",
	"staticMemoGated", "staticMemoStateRead", "staticObjs", "staticVersion",
	"staticZonesEp", "tape", "trigGrant", "trigZones", "trigZonesEp",
	"turnStartEpoch", "turnsTakenEpoch", "typesEpoch", "typesIncrReady",
	"typesMayDiffer", "typesObjs", "typesProbe", "typesProbeEpoch",
	"typesProbeObjs", "typesProbeReady", "typesProbeTrue", "typesProbeVersion",
	"typesSelfOnly", "typesSrcs", "typesVersion", "walkObjCls", "windowDiagnostics",
}

package divergence

import (
	"fmt"
	"sort"

	"github.com/adams-shaun/gorge/engine/decision"
)

// Material reports whether the two answers a and b to decision d differ
// materially (design §5.5): a true result is a divergence worth capturing,
// and the returned string is its materiality reason (§8.2's review
// ordering). The predicate is deliberately conservative toward material —
// a false "material" costs one triage, a false "equivalent" loses data
// silently.
//
// Equivalences, in the order they are tested:
//
//   - a payment-only difference (same chosen action, different plan
//     witness) — a payment plan is not a strategic choice;
//   - equal choices, as sets for unordered kinds and as sequences for
//     ordered ones (arrange, trigger order, a repeatable modes answer);
//   - name equivalence: chosen options that map to the same canonical
//     tuple (option kind, name, ability index, svar, mode, amount, mana
//     symbol, counter, player, attacker, battle, keyword, cost, required)
//     are the same choice — tapping the left Mountain instead of the right
//     one is not a divergence. Names come from the decision's own options,
//     which is the seat's own knowledge at that decision;
//   - a priority decision whose two answers are all bare mana-source
//     activations (the same test as searchseat's bareManaSources).
//
// Intent.Rest (the arrange pile-B order) is part of the answer: two arrange
// answers with the same chosen set are equivalent only when their pile-B
// orders name the same options in the same order too.
func Material(d *decision.Decision, a, b decision.Intent) (bool, string) {
	if d == nil {
		return false, ""
	}

	// One side acts through the payment extension and the other does not:
	// the chosen option differs, so the difference is material no matter
	// what the (necessarily empty) choice lists say.
	if (a.Payment != nil) != (b.Payment != nil) || (a.Announce != nil) != (b.Announce != nil) {
		return true, materialReason(d, a, b)
	}
	if a.Payment != nil {
		if a.Payment.ActionID == b.Payment.ActionID {
			return false, ""
		}
		return true, materialReason(d, a, b)
	}
	if a.Announce != nil {
		if a.Announce.ActionID == b.Announce.ActionID {
			return false, ""
		}
		return true, materialReason(d, a, b)
	}

	if answersEqual(d, a, b) {
		return false, ""
	}
	// Two answers that both consist only of bare mana-source taps name the
	// same strategic move (float mana), whichever sources they use.
	if d.Kind == decision.KPriority && allBareManaSources(d, a) && allBareManaSources(d, b) {
		return false, ""
	}
	return true, materialReason(d, a, b)
}

// materialReason classifies a material difference for the review ordering
// (design §5.5): cast-vs-pass > attack-set > block-set > target > mode >
// other.
func materialReason(d *decision.Decision, a, b decision.Intent) string {
	switch d.Kind {
	case decision.KAttackers:
		return MaterialAttackSet
	case decision.KBlockers:
		return MaterialBlockSet
	case decision.KTarget:
		return MaterialTarget
	case decision.KModes:
		return MaterialMode
	case decision.KPriority:
		if emptyAnswer(a) || emptyAnswer(b) {
			return MaterialCastVsPass
		}
		return MaterialOther
	default:
		return MaterialOther
	}
}

// emptyAnswer reports whether an intent answers nothing at all: no choice,
// no payment action, no announcement — a pass.
func emptyAnswer(in decision.Intent) bool {
	return len(in.Choices) == 0 && len(in.Rest) == 0 && in.Payment == nil && in.Announce == nil
}

// allBareManaSources reports whether every chosen option is a priority
// window's bare mana-source activation ("activate" with no cost), the same
// test searchseat's bareManaSources applies.
func allBareManaSources(d *decision.Decision, in decision.Intent) bool {
	if emptyAnswer(in) {
		return false
	}
	for _, c := range in.Choices {
		if c < 0 || c >= len(d.Options) {
			return false
		}
		o := d.Options[c]
		if o.Kind != "activate" || o.Cost != "" {
			return false
		}
	}
	return true
}

// orderedKind reports whether the choice order itself is part of the
// answer: an arrange answer's pile order, a trigger-order answer's
// sequence, and a repeatable modes answer's chosen-mode order (CR 601.2b).
func orderedKind(d *decision.Decision) bool {
	return d.Kind == decision.KArrange || d.Kind == decision.KTriggerOrder || d.Repeatable
}

// answersEqual compares two intents' choice lists — as sequences for the
// ordered kinds, as multisets otherwise — through the options' canonical
// tuples, and for an arrange answer the pile-B order as well.
func answersEqual(d *decision.Decision, a, b decision.Intent) bool {
	if orderedKind(d) {
		if !sequencesEqual(d, a.Choices, b.Choices) {
			return false
		}
		if d.Kind == decision.KArrange && !sequencesEqual(d, a.Rest, b.Rest) {
			return false
		}
		return true
	}
	return multisetsEqual(d, a.Choices, b.Choices)
}

// sequencesEqual compares two index sequences element-wise through the
// options' canonical tuples.
func sequencesEqual(d *decision.Decision, a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		ka, okA := optionKey(d, a[i])
		kb, okB := optionKey(d, b[i])
		if !okA || !okB || ka != kb {
			return false
		}
	}
	return true
}

// multisetsEqual compares two index sets through the options' canonical
// tuples, ignoring order: two answers naming the same named things in a
// different order are the same answer on an unordered kind.
func multisetsEqual(d *decision.Decision, a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	ka := optionKeys(d, a)
	kb := optionKeys(d, b)
	if ka == nil || kb == nil {
		return false
	}
	sort.Strings(ka)
	sort.Strings(kb)
	for i := range ka {
		if ka[i] != kb[i] {
			return false
		}
	}
	return true
}

func optionKeys(d *decision.Decision, cs []int) []string {
	keys := make([]string, 0, len(cs))
	for _, c := range cs {
		k, ok := optionKey(d, c)
		if !ok {
			return nil
		}
		keys = append(keys, k)
	}
	return keys
}

// optionKey renders one option's canonical tuple (design §5.5): option
// kind, name, ability index, mode, amount, plus the discriminating fields
// (mana symbol, counter, player, attacker, battle, ability anchor, required
// flag) — everything that distinguishes one choice from another except the
// object id and the list position, so two same-named lands or creatures
// collapse together. ok is false when the index is out of range, in which
// case the caller falls back to raw index comparison semantics (a nil or
// mismatched key set never compares equal).
func optionKey(d *decision.Decision, c int) (string, bool) {
	if c < 0 || c >= len(d.Options) {
		return "", false
	}
	o := d.Options[c]
	return fmt.Sprintf("%s\x1f%s\x1f%d\x1f%s\x1f%s\x1f%d\x1f%s\x1f%s\x1f%d\x1f%d\x1f%d\x1f%s\x1f%s\x1f%t",
		o.Kind, o.Label, o.Ability, o.SVar, o.Mode, o.Amount, o.ManaSymbol, o.Counter,
		o.Player, o.Attacker, o.Battle, o.Keyword, o.Cost, o.Required), true
}

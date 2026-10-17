package divergence

import (
	"encoding/json"
	"errors"
	"io"
	"sort"

	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/engine/state"
)

// The JSONL record shapes of the human-divergence capture (design §5.6,
// §6.1). Both are append-only wire formats of the same kind as the feedback
// snapshot: never replay input, never an event, read and written only by
// the capture and the offline tools.

const (
	// RecordTypeDivergence names a divergences.jsonl line.
	RecordTypeDivergence = "divergence-v1"
	// RecordTypeTriage names a triage.jsonl line.
	RecordTypeTriage = "triage-v1"

	// DivergenceSchemaVersion and TriageSchemaVersion are the two records'
	// schema versions. A field added later bumps the version and keeps the
	// old line readable.
	DivergenceSchemaVersion = 1
	TriageSchemaVersion     = 1
)

// Materiality reasons (design §5.5). The scanner records one on every
// divergence and the review ranking orders by it (§8.2).
const (
	MaterialCastVsPass = "cast-vs-pass"
	MaterialAttackSet  = "attack-set"
	MaterialBlockSet   = "block-set"
	MaterialTarget     = "target"
	MaterialMode       = "mode"
	MaterialOther      = "other"
)

// Untriaged statuses (design §6.2): why a divergence's teacher verdict is
// missing. untriaged records are counted, never exported.
const (
	// StatusShadowDiverged: the recomputed shadow answer B' differs from
	// the recorded B — a corpus pin move, a policy change, or a
	// determinism defect.
	StatusShadowDiverged = "shadow_diverged"
	// StatusSnapshotPartial: the capture was written under the semantic
	// partial rule (tokens unread), so the replay is not faithful.
	StatusSnapshotPartial = "snapshot_partial"
)

// Indistinguishable sub-buckets (design §6.2), from the omniscient arm.
const (
	// SubBucketHiddenInfo: the omniscient arm clears in either direction —
	// the right answer depended on hidden cards.
	SubBucketHiddenInfo = "hidden_info"
	// SubBucketReasoningOrTie: the omniscient arm does not clear either —
	// a true tie, or a difference the teacher's rollouts cannot resolve.
	SubBucketReasoningOrTie = "reasoning_or_tie"
)

// Bucket is the pre-registered triage verdict (design §6.2).
type Bucket string

const (
	// BucketHumanBetter: the teacher sides with the human — a bot defect.
	BucketHumanBetter Bucket = "human_better"
	// BucketBotBetter: the teacher sides with the bot — a human error, or
	// a hold or plan the teacher cannot value (§6.3).
	BucketBotBetter Bucket = "bot_better"
	// BucketIndistinguishable: the teacher cannot separate the two.
	BucketIndistinguishable Bucket = "indistinguishable"
	// BucketUntriaged: no verdict — excluded from all exports, counted.
	BucketUntriaged Bucket = "untriaged"
)

// Answer is one recorded side of a divergence: the choices the side
// submitted, and the rendered summary of what they mean. Choices indexes
// the decision's own options, so the record stays seat-private on its own.
type Answer struct {
	Choices []int  `json:"choices"`
	Label   string `json:"label"`
}

// Divergence is one line of divergences.jsonl (design §5.6): one place
// where the human's answer and the shadow's differ materially. event_seq is
// the last event before the decision was answered, so the review UI can
// open the seat's historical view there.
type Divergence struct {
	RecordType    string            `json:"record_type"`
	SchemaVersion int               `json:"schema_version"`
	ID            string            `json:"id"`
	Table         string            `json:"table"`
	Match         int               `json:"match"`
	Seat          state.PlayerID    `json:"seat"`
	Intent        int               `json:"intent"`
	EventSeq      uint64            `json:"event_seq"`
	DecisionSeq   uint64            `json:"decision_seq"`
	Turn          int               `json:"turn"`
	Kind          decision.Kind     `json:"kind"`
	Policy        string            `json:"policy"`
	ShadowSeed    uint64            `json:"shadow_seed"`
	Options       []decision.Option `json:"options"`
	Human         Answer            `json:"human"`
	Shadow        Answer            `json:"shadow"`
	Material      string            `json:"material"`
}

// Provenance records what a triage run was built from (design §6.1).
type Provenance struct {
	Commit   string `json:"commit,omitempty"`
	ForgeRef string `json:"forge_ref,omitempty"`
}

// Triage is one line of triage.jsonl (design §6.1): the offline teacher's
// verdict over one divergence. Evaluations carries each arm's evaluation
// JSON (keyed by arm name — "honest", "omniscient") verbatim; the records
// keep the arms as raw JSON and export-labels reads only the honest arm.
type Triage struct {
	RecordType    string         `json:"record_type"`
	SchemaVersion int            `json:"schema_version"`
	ID            string         `json:"id"`
	Table         string         `json:"table,omitempty"`
	Match         int            `json:"match,omitempty"`
	Seat          state.PlayerID `json:"seat,omitempty"`
	Intent        int            `json:"intent"`
	Bucket        Bucket         `json:"bucket"`
	SubBucket     string         `json:"sub_bucket,omitempty"`
	Status        string         `json:"status,omitempty"`
	Material      string         `json:"material,omitempty"`
	// PlanFlag marks a bot_better divergence the player explained with a
	// plan (§6.3, §8.5's turn_plan/opp_behaviour ticks). The default DPO
	// export excludes flagged pairs.
	PlanFlag bool `json:"plan_flag,omitempty"`
	// Candidates are the evaluated answers, shadow first, as recorded.
	Candidates []Answer `json:"candidates,omitempty"`
	// Evaluations maps the arm name to that arm's evaluation JSON.
	// encoding/json writes map keys sorted, so the line stays deterministic.
	Evaluations map[string]json.RawMessage `json:"evaluations,omitempty"`
	Provenance  Provenance                 `json:"provenance,omitempty"`
}

// sortKey orders records by intent index, the file order both formats
// promise (design §5.6); seat and id break ties deterministically.
func divergenceLess(a, b Divergence) bool {
	if a.Intent != b.Intent {
		return a.Intent < b.Intent
	}
	if a.Seat != b.Seat {
		return a.Seat < b.Seat
	}
	return a.ID < b.ID
}

func triageLess(a, b Triage) bool {
	if a.Intent != b.Intent {
		return a.Intent < b.Intent
	}
	if a.Seat != b.Seat {
		return a.Seat < b.Seat
	}
	return a.ID < b.ID
}

// WriteDivergences writes one compact JSON object per line, sorted by
// intent index. The input slice is copied before sorting: the caller's
// order is never disturbed, and equal-intent records keep a deterministic
// relative order. Lines carry a trailing newline and no other whitespace,
// so write → read → write is byte-identical.
func WriteDivergences(w io.Writer, ds []Divergence) error {
	sorted := make([]Divergence, len(ds))
	copy(sorted, ds)
	sort.SliceStable(sorted, func(i, j int) bool { return divergenceLess(sorted[i], sorted[j]) })
	for _, d := range sorted {
		b, err := json.Marshal(d)
		if err != nil {
			return err
		}
		if _, err := w.Write(append(b, '\n')); err != nil {
			return err
		}
	}
	return nil
}

// ReadDivergences reads a divergences.jsonl stream written by
// WriteDivergences. The file order is preserved; use WriteDivergences to
// re-sort.
func ReadDivergences(r io.Reader) ([]Divergence, error) {
	var ds []Divergence
	dec := json.NewDecoder(r)
	for {
		var d Divergence
		if err := dec.Decode(&d); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		ds = append(ds, d)
	}
	return ds, nil
}

// WriteTriages writes triage records, sorted by intent index, with the
// same line discipline as WriteDivergences.
func WriteTriages(w io.Writer, ts []Triage) error {
	sorted := make([]Triage, len(ts))
	copy(sorted, ts)
	sort.SliceStable(sorted, func(i, j int) bool { return triageLess(sorted[i], sorted[j]) })
	for _, t := range sorted {
		b, err := json.Marshal(t)
		if err != nil {
			return err
		}
		if _, err := w.Write(append(b, '\n')); err != nil {
			return err
		}
	}
	return nil
}

// ReadTriages reads a triage.jsonl stream written by WriteTriages.
func ReadTriages(r io.Reader) ([]Triage, error) {
	var ts []Triage
	dec := json.NewDecoder(r)
	for {
		var t Triage
		if err := dec.Decode(&t); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		ts = append(ts, t)
	}
	return ts, nil
}

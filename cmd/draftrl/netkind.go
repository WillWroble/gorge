package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/adams-shaun/gorge/draftlab"
	"github.com/adams-shaun/gorge/limited/draftfeat"
	"github.com/adams-shaun/gorge/limited/draftnet"
	"github.com/adams-shaun/gorge/limited/pickbot"
)

// The "net" drafter kind (DR-8/DR-9): net:WEIGHTS.dpkw[@TABLE.json] is a
// pickbot.NetDrafter at temperature 0 (hosted play: argmax, seeded
// tie-break). The table defaults to table.json beside the weights, and the
// weights' meta must name that table's hash (draftnet.CheckTable), so a
// net is never run against a table it was not trained on. It serves both
// the rollout league and the gate's -drafters.
func init() { RegisterDrafterKind("net", netKind) }

type loadedNet struct {
	net   *draftnet.Net
	table *draftfeat.SetTable
	ref   draftlab.AgentRef
}

var netCache = struct {
	sync.Mutex
	m map[string]*loadedNet
}{m: map[string]*loadedNet{}}

func loadNet(arg string) (*loadedNet, error) {
	netCache.Lock()
	defer netCache.Unlock()
	if l, ok := netCache.m[arg]; ok {
		return l, nil
	}
	wpath, tpath, ok := strings.Cut(arg, "@")
	if !ok {
		tpath = filepath.Join(filepath.Dir(wpath), "table.json")
	}
	wb, err := os.ReadFile(wpath)
	if err != nil {
		return nil, err
	}
	n, err := draftnet.Load(wb)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", wpath, err)
	}
	tb, err := os.ReadFile(tpath)
	if err != nil {
		return nil, err
	}
	t, err := draftfeat.DecodeSetTable(tb)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", tpath, err)
	}
	if err := n.CheckTable(t); err != nil {
		return nil, fmt.Errorf("%s with %s: %w", wpath, tpath, err)
	}
	sum := sha256.Sum256(wb)
	l := &loadedNet{net: n, table: t, ref: draftlab.AgentRef{Name: "net", Version: hex.EncodeToString(sum[:6])}}
	netCache.m[arg] = l
	return l, nil
}

// netLogitDrafter is a NetDrafter whose recorder keeps the last pick's
// logits for the rollout's LogitSource hook.
type netLogitDrafter struct {
	pickbot.NetDrafter
	last []float32
}

func (d *netLogitDrafter) Record(v pickbot.PickView, f *draftfeat.Features, in *draftnet.Inputs, logits []float32, chosen int) {
	d.last = append(d.last[:0], logits...)
}

func (d *netLogitDrafter) LastLogits() []float32 { return d.last }

func netKind(lab *draftlab.Lab, arg, set string) (pickbot.Drafter, draftlab.AgentRef, error) {
	l, err := loadNet(arg)
	if err != nil {
		return nil, draftlab.AgentRef{}, err
	}
	if l.table.Set != set {
		return nil, draftlab.AgentRef{}, fmt.Errorf("net table is set %s, pod is %s", l.table.Set, set)
	}
	d := &netLogitDrafter{NetDrafter: pickbot.NetDrafter{Net: l.net, Table: l.table}}
	d.Recorder = d
	return d, l.ref, nil
}

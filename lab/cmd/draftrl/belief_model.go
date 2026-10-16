package main

import (
	"fmt"
	"math"
	"math/rand/v2"
	"strconv"
	"strings"
	"sync"

	"github.com/adams-shaun/gorge/lab/draft/draftlab"
	"github.com/adams-shaun/gorge/lab/draft/limited/draftfeat"
	"github.com/adams-shaun/gorge/lab/draft/limited/pickbot"
	"github.com/adams-shaun/gorge/lab/draft/limited/setdata"
)

// modelDrafter picks exactly as draftbelief's opponent model assumes: a
// latent pair drawn at its first pick, then softmax(β(v + κ_k·fit)). It is
// the sanity check for the inference (a model-generated pod must be
// recovered), registered as league kind "model:BETA_KAPPA".
type modelDrafter struct {
	t           *draftfeat.SetTable
	beta, kappa float64
	pair        int
}

var modelTable struct {
	once sync.Once
	t    *draftfeat.SetTable
	err  error
}

func init() { RegisterDrafterKind("model", modelKind) }

func modelKind(lab *draftlab.Lab, arg, set string) (pickbot.Drafter, draftlab.AgentRef, error) {
	b, k, ok := strings.Cut(arg, "_")
	beta, e1 := strconv.ParseFloat(b, 64)
	kappa, e2 := strconv.ParseFloat(k, 64)
	if !ok || e1 != nil || e2 != nil {
		return nil, draftlab.AgentRef{}, fmt.Errorf("model drafter %q: want BETA_KAPPA", arg)
	}
	modelTable.once.Do(func() {
		modelTable.t, modelTable.err = defaultSetTable(lab, set, setdata.DefaultCollation, nil)
	})
	if modelTable.err != nil {
		return nil, draftlab.AgentRef{}, modelTable.err
	}
	return &modelDrafter{t: modelTable.t, beta: beta, kappa: kappa, pair: -1}, draftlab.AgentRef{Name: "model", Version: arg}, nil
}

func (d *modelDrafter) Pick(v pickbot.PickView, r *rand.Rand) string {
	if d.pair < 0 {
		d.pair = r.IntN(draftfeat.NPairs)
	}
	kap := d.kappa
	if v.Round == 1 && v.PickNo <= d.t.Model.KappaRamp {
		kap *= float64(v.PickNo-1) / float64(d.t.Model.KappaRamp)
	}
	w := make([]float64, len(v.Pack))
	for i, c := range v.Pack {
		ci := d.t.Index(c.Name)
		val, fit := d.t.Model.VMissing, 0.5
		if ci >= 0 {
			val = d.t.Cards[ci].V
			fit = fitOf(d.t.Cards[ci].Mask, d.pair)
		}
		w[i] = math.Exp(d.beta * (val + kap*fit))
	}
	tot := 0.0
	for _, x := range w {
		tot += x
	}
	x := r.Float64() * tot
	for i, y := range w {
		x -= y
		if x < 0 {
			return v.Pack[i].ID
		}
	}
	return v.Pack[len(v.Pack)-1].ID
}

func fitOf(mask uint8, a int) float64 {
	n, in := 0, 0
	for i := 0; i < draftfeat.NColours; i++ {
		if mask&(1<<i) != 0 {
			n++
			if draftfeat.PairMask[a]&(1<<i) != 0 {
				in++
			}
		}
	}
	if n == 0 {
		return 0.5
	}
	return float64(in) / float64(n)
}

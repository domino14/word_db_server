package wordvault

import (
	"encoding/json"
	"testing"

	"github.com/matryer/is"
	"github.com/open-spaced-repetition/go-fsrs/v4"
)

func TestApplyFsrsVersion(t *testing.T) {
	is := is.New(t)
	// Parameters as stored by go-fsrs v3: 19 weights and no learning steps.
	v5JSON, err := json.Marshal(fsrs5Weights)
	is.NoErr(err)
	stored := `{"RequestRetention":0.85,"MaximumInterval":1825,"Weights":` +
		string(v5JSON) + `,"Decay":-0.5,"Factor":0.2345679012345678,` +
		`"EnableShortTerm":true,"EnableFuzz":true}`

	var params fsrs.Parameters
	is.NoErr(json.Unmarshal([]byte(stored), &params))
	applyFsrsVersion(&params, FsrsVersion5)
	is.Equal([19]float64(params.W[:19]), fsrs5Weights)
	is.Equal(params.W[20], 0.5)
	is.Equal(params.RequestRetention, 0.85)
	is.Equal(params.MaximumInterval, 1825.0)
	is.True(params.EnableShortTerm)
	is.Equal(params.LearningSteps, fsrs.DefaultLearningSteps())
	is.Equal(params.RelearningSteps, fsrs.DefaultRelearningSteps())

	// The version, not the stored weights, decides the weights.
	applyFsrsVersion(&params, FsrsVersion6)
	is.Equal(params.W, fsrs.DefaultWeights())

	// Explicitly empty steps are kept.
	params.LearningSteps = []float64{}
	applyFsrsVersion(&params, FsrsVersion6)
	is.Equal(len(params.LearningSteps), 0)
	is.True(params.LearningSteps != nil)
}

func TestIntervalDays(t *testing.T) {
	is := is.New(t)
	for _, version := range []int16{FsrsVersion5, FsrsVersion6} {
		for _, retention := range []float64{0.8, 0.9, 0.95} {
			p := fsrs.DefaultParam()
			applyFsrsVersion(&p, version)
			p.RequestRetention = retention
			for _, s := range []float64{3, 50, 400} {
				ivl := intervalDays(p, s)
				// Retrievability crosses the target retention at the interval.
				is.True(p.ForgettingCurve(ivl-1, s) > retention)
				is.True(p.ForgettingCurve(ivl+1, s) < retention)
			}
		}
	}
}

func TestDeckParams(t *testing.T) {
	is := is.New(t)
	global := fsrs.DefaultParam()
	global.EnableFuzz = true
	global.MaximumInterval = 1825
	global.RequestRetention = 0.9
	applyFsrsVersion(&global, FsrsVersion6)

	// No override: the global params.
	p, err := deckParams(nil, global)
	is.NoErr(err)
	is.Equal(p, global)

	// What EditDeck used to store: a whole fsrs.DefaultParam(), with fuzz
	// off, a 100-year maximum interval and no learning steps. Only retention
	// and the scheduler come from it now.
	old := fsrs.DefaultParam()
	old.RequestRetention = 0.85
	old.EnableShortTerm = true
	old.LearningSteps = nil
	oldJSON, err := json.Marshal(old)
	is.NoErr(err)
	p, err = deckParams(oldJSON, global)
	is.NoErr(err)
	is.Equal(p.RequestRetention, 0.85)
	is.True(p.EnableShortTerm)
	is.True(p.EnableFuzz)
	is.Equal(p.MaximumInterval, 1825.0)
	is.Equal(p.W, global.W)
	is.Equal(p.LearningSteps, global.LearningSteps)

	// What EditDeck stores now.
	p, err = deckParams([]byte(`{"RequestRetention":0.95,"EnableShortTerm":false}`), global)
	is.NoErr(err)
	is.Equal(p.RequestRetention, 0.95)
	is.True(!p.EnableShortTerm)
	is.True(p.EnableFuzz)
	is.Equal(p.MaximumInterval, 1825.0)
}

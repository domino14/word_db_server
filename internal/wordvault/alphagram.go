package wordvault

import (
	"slices"
	"strings"

	wglconfig "github.com/domino14/word-golib/config"
	"github.com/domino14/word-golib/tilemapping"
)

// lexiconAlphagram sorts the letters of alpha in the lexicon's tile order,
// which is how stored alphagrams are sorted. A client sorting by code point
// gets non-English letters wrong, e.g. Polish Ń goes right after N, not
// after Z. It returns alpha unchanged if it can't be parsed into tiles.
func lexiconAlphagram(cfg *wglconfig.Config, lexicon, alpha string) string {
	dist, err := tilemapping.ProbableLetterDistribution(cfg, lexicon)
	if err != nil {
		return alpha
	}
	mls, err := tilemapping.ToMachineLetters(strings.ToUpper(alpha), dist.TileMapping())
	if err != nil {
		return alpha
	}
	slices.Sort(mls)
	return tilemapping.MachineWord(mls).UserVisible(dist.TileMapping())
}

// lookupAlphagrams returns the alphagrams to look up for user input: each one
// as given, plus its lexicon-sorted form if that differs.
func lookupAlphagrams(cfg *wglconfig.Config, lexicon string, alphas []string) []string {
	out := make([]string, 0, len(alphas))
	for _, a := range alphas {
		out = append(out, a)
		if sorted := lexiconAlphagram(cfg, lexicon, a); sorted != a {
			out = append(out, sorted)
		}
	}
	return out
}

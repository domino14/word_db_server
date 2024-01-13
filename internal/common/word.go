package common

import (
	"sort"

	"github.com/domino14/word-golib/tilemapping"
)

type Word struct {
	word string
	dist *tilemapping.LetterDistribution
}

func (w Word) MakeAlphagram() string {
	mls, err := tilemapping.ToMachineLetters(w.word, w.dist.TileMapping())
	if err != nil {
		panic(err)
	}
	// Alphagrams should put blank at the end, due to convention.
	sort.Slice(mls, func(i, j int) bool {
		if mls[i] > 0 && mls[j] > 0 {
			return mls[i] < mls[j]
		} else if mls[i] == 0 {
			// blank is never less than j
			return false
		}
		// blank is always greater than i
		return true
	})
	return tilemapping.MachineWord(mls).UserVisible(w.dist.TileMapping())
}

func InitializeWord(word string, dist *tilemapping.LetterDistribution) Word {
	return Word{word, dist}
}

func (w Word) Word() string {
	return w.word // stop saying word so much
}

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
	sort.Slice(mls, func(i, j int) bool {
		return mls[i] < mls[j]
	})
	return tilemapping.MachineWord(mls).UserVisible(w.dist.TileMapping())
}

func InitializeWord(word string, dist *tilemapping.LetterDistribution) Word {
	return Word{word, dist}
}

func (w Word) Word() string {
	return w.word // stop saying word so much
}

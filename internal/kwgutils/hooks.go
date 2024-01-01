// package kwgutils implements some functions that use Andy Kurnia's KWG (Kurnia Word
// Graph) to find things like hooks, reverse hooks, etc.
// Note: word_db_server is not yet multichar-tile aware. These functions need to
// deal entirely with MachineLetters eventually.
package kwgutils

import (
	"github.com/domino14/macondo/kwg"
	"github.com/domino14/macondo/tilemapping"
)

const (
	BackHooks = iota
	FrontHooks
	BackInnerHook
	FrontInnerHook
)

func Reverse(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

func FindHooks(kwg *kwg.KWG, word string, hooktype int, ld *tilemapping.LetterDistribution) []rune {
	mls, err := tilemapping.ToMachineLetters(word, ld.TileMapping())
	if err != nil {
		panic(err)
	}
	nodeIdx := kwg.ArcIndex(1)
	if hooktype == BackHooks {
		// ArcIndex 0 is the dawg, can search directly.
		nodeIdx = kwg.ArcIndex(0)
	} else if hooktype == FrontHooks {
		word = Reverse(word)
	}

	hooks := []rune{}

	lidx := 0
	for {
		if lidx > len(word)-1 {
			// If we've gone too far the word is not found.
			return nil
		}
		letter := word[lidx]
		if kwg.Tile(nodeIdx) == uint8(letter) {
			if lidx == len(word)-1 {
				if kwg.Accepts(nodeIdx) {
					// yay we're here
					break
				}
			}
			nodeIdx = kwg.ArcIndex(nodeIdx)
			lidx++
		} else {
			if kwg.IsEnd(nodeIdx) {
				return nil
			}
			nodeIdx++
		}
	}

	// if we made it here, the word was found. enumerate all next nodes that end.

}

func FindInnerHook(kwg *kwg.KWG, word string, hooktype int) bool {

}

// This package generates anagrams and subanagrams and has an RPC
// interface.
// NOTE: This is a slower version of the dawg_anagrammer in the word-golib dependency.
// However, dawg_anagrammer is deterministic and cannot natively handle range queries
// (i.e. such as [AEIOU]Z) without some work.
package anagrammer

import (
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/rs/zerolog/log"

	"github.com/domino14/word-golib/kwg"
	"github.com/domino14/word-golib/tilemapping"
)

type AnagramMode int

const (
	ModeExact AnagramMode = 0
	ModeBuild AnagramMode = 1
)

type AnagramStruct struct {
	answerList []string
	mode       AnagramMode
	numLetters int
}

type rangeBlank struct {
	count       int
	letterRange []tilemapping.MachineLetter
}

// RackWrapper wraps an tilemapping.Rack and adds helper data structures
// to make it usable for anagramming.
type RackWrapper struct {
	rack        *tilemapping.Rack
	rangeBlanks []rangeBlank
	numLetters  int
}

func makeRack(letters string, alph *tilemapping.TileMapping) (*RackWrapper, error) {

	bracketedLetters := ""
	parsingBracket := false

	regularLetters := ""
	rack := tilemapping.NewRack(alph)

	rb := []rangeBlank{}
	numLetters := 0

	for _, s := range letters {
		if s == '(' {
			// Basically treat as a blank that can only be a subset of all
			// letters.
			if parsingBracket {
				return nil, errors.New("badly formed search string")
			}
			parsingBracket = true
			bracketedLetters = ""
			continue
		}
		if s == ')' {
			if !parsingBracket {
				return nil, errors.New("badly formed search string")
			}
			parsingBracket = false
			mls, err := tilemapping.ToMachineLetters(bracketedLetters, alph)
			if err != nil {
				return nil, err
			}
			rb = append(rb, rangeBlank{1, mls})
			numLetters++
			continue

		}
		if parsingBracket {
			bracketedLetters += string(s)
			continue
		}
		regularLetters += string(s)
	}
	if parsingBracket {
		return nil, errors.New("badly formed search string")
	}
	mls, err := tilemapping.ToMachineLetters(regularLetters, alph)
	if err != nil {
		return nil, err
	}
	rack.Set(mls)
	numLetters += len(mls)

	return &RackWrapper{
		rack:        rack,
		rangeBlanks: rb,
		numLetters:  numLetters,
	}, nil
}

func Anagram(letters string, d *kwg.KWG, mode AnagramMode) []string {

	letters = strings.ToUpper(letters)
	answerList := []string{}
	alph := d.GetAlphabet()

	rw, err := makeRack(letters, alph)
	if err != nil {
		log.Error().Msgf("Anagram error: %v", err)
		return []string{}
	}

	ahs := &AnagramStruct{
		answerList: answerList,
		mode:       mode,
		numLetters: rw.numLetters,
	}
	stopChan := make(chan struct{})

	go func() {
		// Use the dawg encoded in the KWG - it's at arc index 0.
		anagram(ahs, d, d.ArcIndex(0), "", rw)
		close(stopChan)
	}()
	<-stopChan

	return dedupeAndTransformAnswers(ahs.answerList, alph)
	//return ahs.answerList
}

func dedupeAndTransformAnswers(answerList []string, alph *tilemapping.TileMapping) []string {
	// Use a map to throw away duplicate answers (can happen with blanks)
	// This seems to be significantly faster than allowing the anagramming
	// goroutine to write directly to a map.
	empty := struct{}{}
	answers := make(map[string]struct{})
	for _, answer := range answerList {
		answers[tilemapping.MachineWord(answer).UserVisible(alph)] = empty
	}

	// Turn the answers map into a string array.
	answerStrings := make([]string, len(answers))
	i := 0
	for k := range answers {
		answerStrings[i] = k
		i++
	}
	return answerStrings
}

// XXX: utf8.RuneCountInString is slow, but necessary to support unicode tiles.
func anagramHelper(letter tilemapping.MachineLetter, d *kwg.KWG,
	ahs *AnagramStruct, nodeIdx uint32, answerSoFar string, rw *RackWrapper) {

	// log.Debug().Msgf("Anagram helper called with %v %v", letter, answerSoFar)
	var nextNodeIdx uint32

	if d.InLetterSet(letter, nodeIdx) {
		toCheck := answerSoFar + string(letter)
		if ahs.mode == ModeBuild || (ahs.mode == ModeExact &&
			utf8.RuneCountInString(toCheck) == ahs.numLetters) {

			// log.Debug().Msgf("Appending word %v", toCheck)
			ahs.answerList = append(ahs.answerList, toCheck)
		}
	}

	for i := nodeIdx; ; i++ {
		nextNodeIdx = d.ArcIndex(i)
		if d.Tile(i) == uint8(letter) {
			anagram(ahs, d, nextNodeIdx, answerSoFar+string(letter), rw)
		}

		if d.IsEnd(i) {
			break
		}
	}

}

func anagram(ahs *AnagramStruct, d *kwg.KWG, nodeIdx uint32,
	answerSoFar string, rw *RackWrapper) {

	for idx, val := range rw.rack.LetArr {
		if val == 0 {
			continue
		}
		rw.rack.LetArr[idx]--
		if idx == 0 {
			// log.Debug().Msgf("Blank is NOT range")

			nlet := tilemapping.MachineLetter(d.GetAlphabet().NumLetters())
			for i := tilemapping.MachineLetter(1); i <= nlet; i++ {
				anagramHelper(i, d, ahs, nodeIdx, answerSoFar, rw)
			}

		} else {
			letter := tilemapping.MachineLetter(idx)
			// log.Debug().Msgf("Found regular letter %v", letter)
			anagramHelper(letter, d, ahs, nodeIdx, answerSoFar, rw)
		}

		rw.rack.LetArr[idx]++
	}
	for idx := range rw.rangeBlanks {
		// log.Debug().Msgf("whichblank %v Blank is range, range is %v",
		// 	rw.whichBlank, blank.letterRange)
		if rw.rangeBlanks[idx].count == 0 {
			continue
		}
		rw.rangeBlanks[idx].count--

		for _, ml := range rw.rangeBlanks[idx].letterRange {
			// log.Debug().Msgf("Making blank %v a %v", rw.whichBlank, ml)
			anagramHelper(ml, d, ahs, nodeIdx, answerSoFar, rw)
		}
		rw.rangeBlanks[idx].count++
	}
}

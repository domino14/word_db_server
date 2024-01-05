// This package generates anagrams and subanagrams and has an RPC
// interface.
// NOTE: This is a slower version of the dawg_anagrammer in the macondo dependency.
// However, dawg_anagrammer is deterministic and cannot natively handle range queries
// (i.e. such as [AEIOU]Z) without some work.
package anagrammer

import (
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/domino14/word-golib/tilemapping"
	"github.com/rs/zerolog/log"
)

const BlankPos = tilemapping.MaxAlphabetSize

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
	bracketedLetters := []tilemapping.MachineLetter{}
	parsingBracket := false

	rack := tilemapping.NewRack(alph)

	convertedLetters := []tilemapping.MachineLetter{}
	rb := []rangeBlank{}
	numLetters := 0
	for _, s := range letters {
		if s == tilemapping.BlankToken {
			convertedLetters = append(convertedLetters, 0)
			numLetters++
			continue
		}

		if s == '[' {
			// Basically treat as a blank that can only be a subset of all
			// letters.
			if parsingBracket {
				return nil, errors.New("badly formed search string")
			}
			parsingBracket = true
			bracketedLetters = []tilemapping.MachineLetter{}
			continue
		}
		if s == ']' {
			if !parsingBracket {
				return nil, errors.New("badly formed search string")
			}
			parsingBracket = false
			rb = append(rb, rangeBlank{1, bracketedLetters})
			numLetters++
			continue

		}
		// Otherwise it's just a letter.
		ml, err := alph.Val(s)
		if err != nil {
			// Ignore this error, but log it.
			log.Error().Msgf("Ignored error: %v", err)
			continue
		}
		if parsingBracket {
			bracketedLetters = append(bracketedLetters, ml)
			continue
		}
		numLetters++
		convertedLetters = append(convertedLetters, ml)
	}
	if parsingBracket {
		return nil, errors.New("badly formed search string")
	}
	rack.Set(convertedLetters)

	return &RackWrapper{
		rack:        rack,
		rangeBlanks: rb,
		numLetters:  numLetters,
	}, nil
}

func Anagram(letters string, d *gaddag.SimpleDawg, mode AnagramMode) []string {

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
		anagram(ahs, d, d.GetRootNodeIndex(), "", rw)
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
func anagramHelper(letter tilemapping.MachineLetter, d *gaddag.SimpleDawg,
	ahs *AnagramStruct, nodeIdx uint32, answerSoFar string, rw *RackWrapper) {

	// log.Debug().Msgf("Anagram helper called with %v %v", letter, answerSoFar)
	var nextNodeIdx uint32
	var nextLetter tilemapping.MachineLetter

	if d.InLetterSet(letter, nodeIdx) {
		toCheck := answerSoFar + string(letter)
		if ahs.mode == ModeBuild || (ahs.mode == ModeExact &&
			utf8.RuneCountInString(toCheck) == ahs.numLetters) {

			// log.Debug().Msgf("Appending word %v", toCheck)
			ahs.answerList = append(ahs.answerList, toCheck)
		}
	}

	numArcs := d.NumArcs(nodeIdx)
	for i := byte(1); i <= numArcs; i++ {
		nextNodeIdx, nextLetter = d.ArcToIdxLetter(nodeIdx + uint32(i))
		if letter == nextLetter {
			anagram(ahs, d, nextNodeIdx, answerSoFar+string(letter), rw)
		}
	}
}

func anagram(ahs *AnagramStruct, d *gaddag.SimpleDawg, nodeIdx uint32,
	answerSoFar string, rw *RackWrapper) {

	for idx, val := range rw.rack.LetArr {
		if val == 0 {
			continue
		}
		rw.rack.LetArr[idx]--
		if idx == BlankPos {
			// log.Debug().Msgf("Blank is NOT range")

			nlet := tilemapping.MachineLetter(d.GetAlphabet().NumLetters())
			for i := tilemapping.MachineLetter(0); i < nlet; i++ {
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

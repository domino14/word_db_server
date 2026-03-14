package dbmaker

import (
	"errors"

	"github.com/domino14/word-golib/kwg"
	"github.com/domino14/word-golib/tilemapping"
)

type LexiconInfo struct {
	LexiconName        string
	LexiconFilename    string
	LexiconIndex       uint8
	DescriptiveName    string
	KWG                *kwg.KWG
	LetterDistribution *tilemapping.LetterDistribution
	Difficulties       map[string]int
	Playabilities      map[string]int
	subChooseCombos    [][]uint64
}

type LexiconFamily []*LexiconInfo

type FamilyName string

const (
	FamilyCSW     FamilyName = "CSW"
	FamilyFISE               = "FISE"
	FamilyTWL                = "TWL"
	FamilyOSPS               = "OSPS"
	FamilyDeutsch            = "Deutsch"
	FamilyFrench             = "FRA"
	FamilyNorsk              = "Norsk"
)

type LexiconMap map[FamilyName]LexiconFamily

const (
	CSWOnlySymbol       = "#"
	TWLOnlySymbol       = "$"
	LexiconUpdateSymbol = "+"
)

func (m LexiconMap) GetLexiconInfo(lexiconName string) (*LexiconInfo, error) {
	// Just do a naive linear search.

	for _, f := range m {
		for _, i := range f {
			if i.LexiconName == lexiconName {
				return i, nil
			}
		}
	}
	return nil, errors.New("not found")
}

func (m LexiconMap) familyName(lexiconName string) (FamilyName, error) {
	for fn, f := range m {
		for _, i := range f {
			if i.LexiconName == lexiconName {
				return fn, nil
			}
		}
	}
	return "", errors.New("not found")
}

func (m LexiconMap) newestInFamily(family FamilyName) *LexiconInfo {
	return m[family][len(m[family])-1]
}

func (m LexiconMap) priorLexicon(family FamilyName, lexiconName string) (*LexiconInfo, error) {
	for idx, i := range m[family] {
		if i.LexiconName == lexiconName {
			if idx > 0 {
				return m[family][idx-1], nil
			} else {
				return nil, errors.New("no prior lexicon")
			}
		}
	}
	return nil, errors.New("lexicon not found")
}

// Initialize the LexiconInfo data structure for a new lexicon,
// pre-calculating combinations as necessary.
func (l *LexiconInfo) Initialize() {
	// Adapted from GPL Zyzzyva's calculation code.
	maxFrequency := uint8(0)
	totalLetters := uint8(0)
	r := uint8(1)
	for _, value := range l.LetterDistribution.Distribution() {
		freq := value
		totalLetters += freq
		if freq > maxFrequency {
			maxFrequency = freq
		}
	}
	// Precalculate M choose N combinations
	l.subChooseCombos = make([][]uint64, maxFrequency+1)
	for i := uint8(0); i <= maxFrequency; i, r = i+1, r+1 {
		subList := make([]uint64, maxFrequency+1)
		for j := uint8(0); j <= maxFrequency; j++ {
			if (i == j) || (j == 0) {
				subList[j] = 1.0
			} else if i == 0 {
				subList[j] = 0.0
			} else {
				subList[j] = l.subChooseCombos[i-1][j-1] +
					l.subChooseCombos[i-1][j]
			}
		}
		l.subChooseCombos[i] = subList
	}
}

// Calculate the number of combinations for an alphagram.
func (l *LexiconInfo) Combinations(alphagram string, withBlanks bool) uint64 {
	// Adapted from GPL Zyzzyva's calculation code.
	letters := make([]tilemapping.MachineLetter, 0)
	counts := make([]uint8, 0)
	combos := make([][]uint64, 0)

	alphML, err := tilemapping.ToMachineLetters(alphagram, l.LetterDistribution.TileMapping())
	if err != nil {
		panic(err)
	}

	for _, letter := range alphML {
		foundLetter := false
		for j, ml := range letters {
			if ml == letter {
				counts[j]++
				foundLetter = true
				break
			}
		}
		if !foundLetter {
			letters = append(letters, letter)
			counts = append(counts, 1)
			combos = append(combos,
				l.subChooseCombos[l.LetterDistribution.Distribution()[letter]])

		}
	}
	totalCombos := uint64(0)
	numLetters := len(letters)
	// Calculate combinations with no blanks
	thisCombo := uint64(1)
	for i := 0; i < numLetters; i++ {
		thisCombo *= combos[i][counts[i]]
	}
	totalCombos += thisCombo
	if !withBlanks {
		return totalCombos
	}
	// Calculate combinations with one blank
	for i := 0; i < numLetters; i++ {
		counts[i]--
		thisCombo = l.subChooseCombos[l.LetterDistribution.Distribution()[0]][1]
		for j := 0; j < numLetters; j++ {
			thisCombo *= combos[j][counts[j]]
		}
		totalCombos += thisCombo
		counts[i]++
	}
	// Calculate combinations with two blanks
	for i := 0; i < numLetters; i++ {
		counts[i]--
		for j := i; j < numLetters; j++ {
			if counts[j] == 0 {
				continue
			}
			counts[j]--
			thisCombo = l.subChooseCombos[l.LetterDistribution.Distribution()[0]][2]

			for k := 0; k < numLetters; k++ {
				thisCombo *= combos[k][counts[k]]
			}
			totalCombos += thisCombo
			counts[j]++
		}
		counts[i]++
	}
	return totalCombos
}

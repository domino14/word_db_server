package dbmaker

import (
	"github.com/domino14/macondo/alphabet"
	"github.com/domino14/macondo/gaddag"
)

type LexiconInfo struct {
	LexiconName        string
	LexiconFilename    string
	LexiconIndex       uint8
	DescriptiveName    string
	Gaddag             *gaddag.SimpleGaddag
	LetterDistribution *alphabet.LetterDistribution
	subChooseCombos    [][]uint64
}

// Initialize the LexiconInfo data structure for a new lexicon,
// pre-calculating combinations as necessary.
func (l *LexiconInfo) Initialize() {
	// Adapted from GPL Zyzzyva's calculation code.
	maxFrequency := uint8(0)
	totalLetters := uint8(0)
	r := uint8(1)
	for _, value := range l.LetterDistribution.Distribution {
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
	letters := make([]rune, 0)
	counts := make([]uint8, 0)
	combos := make([][]uint64, 0)
	for _, letter := range alphagram {
		foundLetter := false
		for j, char := range letters {
			if char == letter {
				counts[j]++
				foundLetter = true
				break
			}
		}
		if !foundLetter {
			letters = append(letters, letter)
			counts = append(counts, 1)
			combos = append(combos,
				l.subChooseCombos[l.LetterDistribution.Distribution[letter]])

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
		thisCombo = l.subChooseCombos[l.LetterDistribution.Distribution['?']][1]
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
			thisCombo = l.subChooseCombos[l.LetterDistribution.Distribution['?']][2]

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

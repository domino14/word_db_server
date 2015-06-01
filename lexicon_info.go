package main

//import "fmt"

type LetterDistribution map[rune]uint8

func EnglishLetterDistribution() LetterDistribution {
	dist := map[rune]uint8{
		'A': 9,
		'B': 2,
		'C': 2,
		'D': 4,
		'E': 12,
		'F': 2,
		'G': 3,
		'H': 2,
		'I': 9,
		'J': 1,
		'K': 1,
		'L': 4,
		'M': 2,
		'N': 6,
		'O': 8,
		'P': 2,
		'Q': 1,
		'R': 6,
		'S': 4,
		'T': 6,
		'U': 4,
		'V': 2,
		'W': 2,
		'X': 1,
		'Y': 2,
		'Z': 1,
		'?': 2,
	}
	return LetterDistribution(dist)
}

type LexiconInfo struct {
	lexiconName        string
	gaddagFilename     string
	lexiconIndex       uint8
	descriptiveName    string
	letterDistribution LetterDistribution
	subChooseCombos    [][]uint64
}

// Initialize the LexiconInfo data structure for a new lexicon,
// pre-calculating combinations as necessary.
func (l *LexiconInfo) Initialize() {
	// Translated from GPL Zyzzyva's calculation code.
	maxFrequency := uint8(0)
	totalLetters := uint8(0)
	r := uint8(1)
	for _, value := range l.letterDistribution {
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

func (l *LexiconInfo) Combinations(alphagram string) uint64 {
	// Translated from GPL Zyzzyva's calculation code.
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
				l.subChooseCombos[l.letterDistribution[letter]])

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
	// Calculate combinations with one blank
	for i := 0; i < numLetters; i++ {
		counts[i]--
		thisCombo = l.subChooseCombos[l.letterDistribution['?']][1]
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
			thisCombo = l.subChooseCombos[l.letterDistribution['?']][2]

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

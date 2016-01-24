package dbmaker

import (
	"sort"
)

type Word struct {
	word    string
	dist    LetterDistribution
	letters []rune
}

func (w Word) String() string {
	return w.word
}

func (w Word) Len() int {
	return len(w.letters)
}

func (w Word) Less(i, j int) bool {
	return w.dist.sortOrder[w.letters[i]] < w.dist.sortOrder[w.letters[j]]
}

func (w Word) Swap(i, j int) {
	w.letters[i], w.letters[j] = w.letters[j], w.letters[i]
}

func (w Word) MakeAlphagram() string {
	w.letters = []rune{}
	for _, char := range w.word {
		w.letters = append(w.letters, char)
	}
	sort.Sort(w)
	return string(w.letters)
}

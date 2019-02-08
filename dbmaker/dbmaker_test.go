package dbmaker

import (
	"testing"

	"github.com/domino14/macondo/alphabet"
)

func TestPopulate(t *testing.T) {
	lexInfo := LexiconInfo{
		LexiconName:        "America",
		LexiconIndex:       7,
		DescriptiveName:    "I am America, and so can you.",
		LetterDistribution: alphabet.EnglishLetterDistribution(),
	}
	lexInfo.Initialize()
	defs, alphs := populateAlphsDefs("test_files/mini_america.txt",
		lexInfo.Combinations,
		lexInfo.LetterDistribution)
	if len(alphs["AEINRST"].words) != 2 {
		t.Error("AEINRST should have 2 words, got",
			len(alphs["AEINRST"].words))
	}
	if len(defs) != 3 {
		t.Error("Defs should have 3 words, got", len(defs))
	}
}

func TestSortedHooks(t *testing.T) {
	lexInfo := LexiconInfo{
		LexiconName:        "FISE09",
		LetterDistribution: alphabet.SpanishLetterDistribution(),
	}
	lexInfo.Initialize()
	hooks := []rune("2ANRSXZ")
	if sortedHooks(hooks, lexInfo.LetterDistribution) != "A2NRSXZ" {
		t.Error("Sorted hooks wrong")
	}
}

type alphaTestCase struct {
	alphagram string
	expected  uint8
}

func TestPointValue(t *testing.T) {
	ptTestCases := []alphaTestCase{
		alphaTestCase{"AEKLOVZ", 23},
		alphaTestCase{"AVYYZZZ", 43},
		alphaTestCase{"AEILNOR", 7},
		alphaTestCase{"DEUTERANOMALIES", 18},
		alphaTestCase{"THE", 6},
		alphaTestCase{"QUICK", 20},
		alphaTestCase{"BROWN", 10},
		alphaTestCase{"FOX", 13},
		alphaTestCase{"JUMPED", 18},
		alphaTestCase{"OVER", 7},
		alphaTestCase{"LAZY", 16},
		alphaTestCase{"DOG", 5},
	}
	for _, tc := range ptTestCases {
		a := &Alphagram{nil, 0, tc.alphagram, 0, 0, 0}
		pts := a.pointValue(alphabet.EnglishLetterDistribution())
		if pts != tc.expected {
			t.Errorf("Expected %d, actual %d, alphagram %s", tc.expected,
				pts, a.alphagram)
		}
	}
}

func TestNumVowels(t *testing.T) {
	vowelTestCases := []alphaTestCase{
		alphaTestCase{"AEKLOVZ", 3},
		alphaTestCase{"AVYYZZZ", 1},
		alphaTestCase{"AEILNOR", 4},
		alphaTestCase{"DEUTERANOMALIES", 8},
		alphaTestCase{"GLYCYLS", 0},
		alphaTestCase{"EUOUAE", 6},
	}
	for _, tc := range vowelTestCases {
		a := &Alphagram{nil, 0, tc.alphagram, 0, 0, 0}
		pts := a.numVowels()
		if pts != tc.expected {
			t.Errorf("Expected %d, actual %d, alphagram %s", tc.expected,
				pts, a.alphagram)
		}
	}
}

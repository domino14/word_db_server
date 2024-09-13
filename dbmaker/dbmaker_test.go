package dbmaker

import (
	"os"
	"testing"

	"github.com/domino14/word-golib/config"
	"github.com/domino14/word-golib/tilemapping"
)

var DefaultConfig = &config.Config{
	DataPath: os.Getenv("WDB_DATA_PATH"),
}

func TestPopulate(t *testing.T) {
	ld, err := tilemapping.GetDistribution(DefaultConfig, "english")
	if err != nil {
		t.Error(err)
	}

	lexInfo := LexiconInfo{
		LexiconName:        "America",
		LexiconIndex:       7,
		DescriptiveName:    "I am America, and so can you.",
		LetterDistribution: ld,
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

type alphaTestCase struct {
	alphagram string
	expected  int
}

func TestPointValue(t *testing.T) {
	ld, err := tilemapping.GetDistribution(DefaultConfig, "english")
	if err != nil {
		t.Error(err)
	}
	ptTestCases := []alphaTestCase{
		{"AEKLOVZ", 23},
		{"AVYYZZZ", 43},
		{"AEILNOR", 7},
		{"DEUTERANOMALIES", 18},
		{"THE", 6},
		{"QUICK", 20},
		{"BROWN", 10},
		{"FOX", 13},
		{"JUMPED", 18},
		{"OVER", 7},
		{"LAZY", 16},
		{"DOG", 5},
	}
	for _, tc := range ptTestCases {
		a := &Alphagram{nil, 0, tc.alphagram, 0, 0, 0}
		pts := a.pointValue(ld)
		if pts != tc.expected {
			t.Errorf("Expected %d, actual %d, alphagram %s", tc.expected,
				pts, a.alphagram)
		}
	}
}

func TestNumVowels(t *testing.T) {
	ld, err := tilemapping.GetDistribution(DefaultConfig, "english")
	if err != nil {
		t.Error(err)
	}
	vowelTestCases := []alphaTestCase{
		{"AEKLOVZ", 3},
		{"AVYYZZZ", 1},
		{"AEILNOR", 4},
		{"DEUTERANOMALIES", 8},
		{"GLYCYLS", 0},
		{"EUOUAE", 6},
	}
	for _, tc := range vowelTestCases {
		a := &Alphagram{nil, 0, tc.alphagram, 0, 0, 0}
		pts := a.numVowels(ld)
		if pts != tc.expected {
			t.Errorf("Expected %d, actual %d, alphagram %s", tc.expected,
				pts, a.alphagram)
		}
	}
}

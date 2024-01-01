package dbmaker

import (
	"testing"

	"github.com/domino14/macondo/tilemapping"
)

type combinationstestpair struct {
	alphagram    string
	combinations uint64
}

var combinationsTests = []combinationstestpair{
	{"AADEEEILMNORSTU", 640342278144},
	{"AAJQQ", 153},
	{"ACEIORT", 2323512},
	{"MMSUUUU", 120},
	{"AIJNORT", 817236},
	{"AEFFGINR", 1077300},
	{"ADEINOPRTTVZ", 860575104},
	{"ABEIPRSTZ", 5669136},
}

var spanishCombinationsTests = []combinationstestpair{
	// This is not actually an alphagram. Testing handling of runes.
	{"OOÑ22", 153},
}

func TestCalcCombinations(t *testing.T) {
	ld, err := tilemapping.GetDistribution(&DefaultConfig, "english")
	if err != nil {
		t.Error(err)
	}

	lexInfo := LexiconInfo{
		LexiconName:        "OWL2",
		LexiconFilename:    "./blah.txt",
		LexiconIndex:       4,
		DescriptiveName:    "American 06",
		LetterDistribution: ld}
	lexInfo.Initialize()

	for _, pair := range combinationsTests {
		combinations := lexInfo.Combinations(pair.alphagram, true)
		if combinations != pair.combinations {
			t.Error("For", pair.alphagram, "expected", pair.combinations,
				"got", combinations)
		}

	}
}

func BenchmarkCombinations(b *testing.B) {
	ld, err := tilemapping.GetDistribution(&DefaultConfig, "english")
	if err != nil {
		b.Error(err)
	}
	lexInfo := LexiconInfo{
		LexiconName:        "OWL2",
		LexiconFilename:    "./blah.txt",
		LexiconIndex:       4,
		DescriptiveName:    "American 06",
		LetterDistribution: ld}
	lexInfo.Initialize()
	for i := 0; i < b.N; i++ {
		lexInfo.Combinations("AEHINORST", true)
	}
}

func TestSpanishCombos(t *testing.T) {
	ld, err := tilemapping.GetDistribution(&DefaultConfig, "spanish")
	if err != nil {
		t.Error(err)
	}
	lexInfo := LexiconInfo{
		LexiconName:        "FISE09",
		LetterDistribution: ld}
	lexInfo.Initialize()

	for _, pair := range spanishCombinationsTests {
		combinations := lexInfo.Combinations(pair.alphagram, true)
		if combinations != pair.combinations {
			t.Error("For", pair.alphagram, "expected", pair.combinations,
				"got", combinations)
		}

	}
}

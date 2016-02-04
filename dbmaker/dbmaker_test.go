package dbmaker

import "testing"
import "github.com/domino14/macondo/lexicon"

func TestPopulate(t *testing.T) {
	lexInfo := lexicon.LexiconInfo{
		LexiconName:        "America",
		LexiconIndex:       7,
		DescriptiveName:    "I am America, and so can you.",
		LetterDistribution: lexicon.EnglishLetterDistribution(),
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
	lexInfo := lexicon.LexiconInfo{
		LexiconName:        "FISE",
		LetterDistribution: lexicon.SpanishLetterDistribution(),
	}
	lexInfo.Initialize()
	hooks := []rune("2ANRSXZ")
	if sortedHooks(hooks, lexInfo.LetterDistribution) != "A2NRSXZ" {
		t.Error("Sorted hooks wrong")
	}
}

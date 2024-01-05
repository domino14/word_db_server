package anagrammer

import (
	"path/filepath"
	"testing"

	"github.com/rs/zerolog/log"

	"github.com/domino14/macondo/config"
	"github.com/domino14/word-golib/tilemapping"
	"github.com/matryer/is"
)

var DefaultConfig = config.DefaultConfig()

type testpair struct {
	rack string
	num  int
}

var buildTests = []testpair{
	{"aehilort", 275},
	{"CINEMATOGRAPHER", 3142},
	{"AEINRST", 276},
	{"KERYGMA", 92},
	{"LOCOFOCO", 16},
	{"VIVIFIC", 2},
	{"ZYZZYVA", 6},
	{"HHHHHHHH", 0},
	{"OCTOROON", 36},
	{"FIREFANG????", 56184},
	{"AEINST??", 9650},
	{"ZZZZ?", 4},
	{"???", 1186},
}
var exactTests = []testpair{
	{"AEHILORT", 1},
	{"CINEMATOGRAPHER", 1},
	{"KERYGMA", 1},
	{"LOCOFOCO", 1},
	{"VIVIFIC", 1},
	{"ZYZZYVA", 1},
	{"HHHHHHHH", 0},
	{"OCTOROON", 1},
	{"FIREFANG????", 2},
	{"AEINST??", 264},
	{"ZZZZ?", 0},
	{"???", 1081},
}

var spanishBuildTests = []testpair{
	{"AEHILORT", 313},
	{"CINEMATOGRAPHER", 7765},
	{"KERYGMA", 42}, // K is not in spanish alphabet though
	{"LOCOFOCO", 14},
	{"VIVIFIC", 3},
	{"123?????", 21808},
	{"ÑUBLADO", 65},
	{"CA1AÑEA", 25},
	{"WKWKKWKWWK", 0},
}

var spanishExactTests = []testpair{
	{"AEHILORT", 0},
	{"CINEMATOGRAPHER", 0},
	{"KERYGMA", 0}, // K is not in spanish alphabet though
	{"LOCOFOCO", 0},
	{"ACENORS", 26}, //!
	{"VIVIFIC", 0},
	{"123?????", 3},
	{"ÑUBLADO", 1},
	{"CA1AÑEA", 1},
	{"CA1AÑEA?", 4},
	{"WKWKWWKWKWKW", 0},
}

type wordtestpair struct {
	rack    string
	answers map[string]struct{}
}

var simpleAnagramTests = []wordtestpair{
	{"AEHILORT", wordlistToSet([]string{"AEROLITH"})},
	{"ADEEMMO?", wordlistToSet([]string{"HOMEMADE", "GAMODEME"})},
	// {"X?", wordlistToSet([]string{"AX", "EX", "XI", "OX", "XU"})},
	{"UX", wordlistToSet([]string{"XU"})},
}

func wordlistToSet(wl []string) map[string]struct{} {
	m := make(map[string]struct{})
	for _, w := range wl {
		m[w] = struct{}{}
	}
	return m
}

func TestAnagram(t *testing.T) {
	path := filepath.Join(DefaultConfig.LexiconPath, "dawg", "America.dawg")
	d, _ := gaddag.LoadDawg(path)
	for _, pair := range buildTests {
		answers := Anagram(pair.rack, d, ModeBuild)
		if len(answers) != pair.num {
			t.Error("For", pair.rack, "expected", pair.num, "got", len(answers), answers)
		}
	}
	for _, pair := range exactTests {
		answers := Anagram(pair.rack, d, ModeExact)
		if len(answers) != pair.num {
			t.Error("For", pair.rack, "expected", pair.num, "got", len(answers), answers)
		}
	}

}

func TestAnagramSpanish(t *testing.T) {
	path := filepath.Join(DefaultConfig.LexiconPath, "dawg", "FISE2.dawg")
	d, _ := gaddag.LoadDawg(path)
	for _, pair := range spanishBuildTests {
		answers := Anagram(pair.rack, d, ModeBuild)
		if len(answers) != pair.num {
			t.Error("For", pair.rack, "expected", pair.num, "got", len(answers))
		}
	}
	for _, pair := range spanishExactTests {
		answers := Anagram(pair.rack, d, ModeExact)
		if len(answers) != pair.num {
			t.Error("For", pair.rack, "expected", pair.num, "got", len(answers))
		}
	}
}

func BenchmarkAnagramBlanks(b *testing.B) {
	// ~ 21.33 ms per op on my macbook pro.
	path := filepath.Join(DefaultConfig.LexiconPath, "dawg", "CSW15.dawg")
	d, _ := gaddag.LoadDawg(path)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Anagram("RETINA??", d, ModeExact)
	}
}

func BenchmarkAnagramFourBlanks(b *testing.B) {
	// ~ 453.6ms
	path := filepath.Join(DefaultConfig.LexiconPath, "dawg", "America.dawg")
	d, _ := gaddag.LoadDawg(path)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Anagram("AEINST????", d, ModeExact)
	}
}

func TestBuildFourBlanks(t *testing.T) {
	path := filepath.Join(DefaultConfig.LexiconPath, "dawg", "America.dawg")
	d, _ := gaddag.LoadDawg(path)
	answers := Anagram("AEINST????", d, ModeBuild)
	expected := 61711
	if len(answers) != expected {
		t.Errorf("Expected %v answers, got %v", expected, len(answers))
	}
}

func TestAnagramFourBlanks(t *testing.T) {
	path := filepath.Join(DefaultConfig.LexiconPath, "dawg", "America.dawg")
	d, _ := gaddag.LoadDawg(path)
	answers := Anagram("AEINST????", d, ModeExact)
	expected := 863
	if len(answers) != expected {
		t.Errorf("Expected %v answers, got %v", expected, len(answers))
	}
}

func TestMakeRack(t *testing.T) {
	rack := "AE[JQXZ]NR?[KY]?"
	alph := tilemapping.EnglishAlphabet()
	rw, err := makeRack(rack, alph)
	is := is.New(t)
	is.NoErr(err)
	is.Equal(rw.rack, tilemapping.RackFromString("AENR??", alph))
	is.Equal(rw.numLetters, 8)
	is.Equal(rw.rangeBlanks, []rangeBlank{
		{1, []tilemapping.MachineLetter{9, 16, 23, 25}},
		{1, []tilemapping.MachineLetter{10, 24}},
	})
}

func TestAnagramRangeSmall(t *testing.T) {
	is := is.New(t)
	path := filepath.Join(DefaultConfig.LexiconPath, "dawg", "CSW19.dawg")
	d, _ := gaddag.LoadDawg(path)
	answers := Anagram("[JQXZ]A", d, ModeExact)
	log.Info().Msgf("answers: %v", answers)

	is.Equal(len(answers), 3)
}

func TestAnagramRangeSmall2(t *testing.T) {
	is := is.New(t)
	path := filepath.Join(DefaultConfig.LexiconPath, "dawg", "CSW19.dawg")
	d, _ := gaddag.LoadDawg(path)
	answers := Anagram("[AEIOU][JQXZ]", d, ModeExact)
	log.Info().Msgf("answers: %v", answers)

	is.Equal(len(answers), 11)
}

func TestAnagramRangeSmallOrderDoesntMatter(t *testing.T) {
	is := is.New(t)
	path := filepath.Join(DefaultConfig.LexiconPath, "dawg", "CSW19.dawg")
	d, _ := gaddag.LoadDawg(path)
	answers := Anagram("[JQXZ][AEIOU]", d, ModeExact)
	log.Info().Msgf("answers: %v", answers)

	is.Equal(len(answers), 11)
}

func TestAnagramRange(t *testing.T) {
	is := is.New(t)
	path := filepath.Join(DefaultConfig.LexiconPath, "dawg", "CSW19.dawg")
	d, _ := gaddag.LoadDawg(path)
	answers := Anagram("AE[JQXZ]NR?[KY]?", d, ModeExact)
	log.Info().Msgf("answers: %v", answers)

	is.Equal(len(answers), 8)
}

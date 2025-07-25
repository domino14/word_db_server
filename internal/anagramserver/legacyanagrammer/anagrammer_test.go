package anagrammer

import (
	"fmt"
	"os"
	"testing"

	"github.com/rs/zerolog/log"

	"github.com/domino14/word-golib/config"
	"github.com/domino14/word-golib/kwg"
	"github.com/domino14/word-golib/tilemapping"
	"github.com/matryer/is"
)

var DefaultConfig = &config.Config{
	DataPath: os.Getenv("WDB_DATA_PATH"),
}

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
	{"KERYGMA", 0}, // K is not in spanish alphabet though
	{"LOCOFOCO", 14},
	{"VIVIFIC", 3},
	{"[CH][LL][RR]?????", 21808},
	{"ÑUBLADO", 65},
	{"CA[CH]AÑEA", 25},
	{"WKWKKWKWWK", 0},
}

var spanishExactTests = []testpair{
	{"AEHILORT", 0},
	{"CINEMATOGRAPHER", 0},
	{"KERYGMA", 0}, // K is not in spanish alphabet though
	{"LOCOFOCO", 0},
	{"ACENORS", 26}, //!
	{"VIVIFIC", 0},
	{"[CH][LL][RR]?????", 3},
	{"ÑUBLADO", 1},
	{"CA[CH]AÑEA", 1},
	{"CA[CH]AÑEA?", 4},
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
	is := is.New(t)

	d, err := kwg.GetKWG(DefaultConfig, "America")
	is.NoErr(err)

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
	is := is.New(t)
	d, err := kwg.GetKWG(DefaultConfig, "FISE2")
	is.NoErr(err)
	for _, pair := range spanishBuildTests {
		answers := Anagram(pair.rack, d, ModeBuild)
		if len(answers) != pair.num {
			t.Error("For", pair.rack, "expected", pair.num, "got", len(answers))
		}
	}
	fmt.Println("exact tests")
	for _, pair := range spanishExactTests {
		answers := Anagram(pair.rack, d, ModeExact)
		if len(answers) != pair.num {
			t.Error("For", pair.rack, "expected", pair.num, "got", len(answers))
		}
	}
}

func BenchmarkAnagramBlanks(b *testing.B) {
	// ~ 21.33 ms per op on my macbook pro.
	is := is.New(b)
	d, err := kwg.GetKWG(DefaultConfig, "CSW15")
	is.NoErr(err)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Anagram("RETINA??", d, ModeExact)
	}
}

func BenchmarkAnagramFourBlanks(b *testing.B) {
	// ~ 453.6ms
	is := is.New(b)
	d, err := kwg.GetKWG(DefaultConfig, "America")
	is.NoErr(err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Anagram("AEINST????", d, ModeExact)
	}
}

func TestBuildFourBlanks(t *testing.T) {
	is := is.New(t)
	d, err := kwg.GetKWG(DefaultConfig, "America")
	is.NoErr(err)
	answers := Anagram("AEINST????", d, ModeBuild)
	expected := 61711
	if len(answers) != expected {
		t.Errorf("Expected %v answers, got %v", expected, len(answers))
	}
}

func TestAnagramFourBlanks(t *testing.T) {
	is := is.New(t)
	d, err := kwg.GetKWG(DefaultConfig, "America")
	is.NoErr(err)
	answers := Anagram("AEINST????", d, ModeExact)
	expected := 863
	if len(answers) != expected {
		t.Errorf("Expected %v answers, got %v", expected, len(answers))
	}
}

func TestMakeRack(t *testing.T) {
	rack := "AE(JQXZ)NR?(KY)?"
	is := is.New(t)

	ld, err := tilemapping.NamedLetterDistribution(DefaultConfig, "english")
	is.NoErr(err)
	rw, err := makeRack(rack, ld.TileMapping())
	is.NoErr(err)

	is.Equal(rw.rack, tilemapping.RackFromString("AENR??", ld.TileMapping()))
	is.Equal(rw.numLetters, 8)
	is.Equal(rw.rangeBlanks, []rangeBlank{
		{1, []tilemapping.MachineLetter{10, 17, 24, 26}},
		{1, []tilemapping.MachineLetter{11, 25}},
	})
}

func TestAnagramRangeSmall(t *testing.T) {
	is := is.New(t)
	d, err := kwg.GetKWG(DefaultConfig, "CSW19")
	is.NoErr(err)
	answers := Anagram("(JQXZ)A", d, ModeExact)
	log.Info().Msgf("answers: %v", answers)

	is.Equal(len(answers), 3)
}

func TestAnagramRangeSmall2(t *testing.T) {
	is := is.New(t)
	d, err := kwg.GetKWG(DefaultConfig, "CSW19")
	is.NoErr(err)
	answers := Anagram("(AEIOU)(JQXZ)", d, ModeExact)
	log.Info().Msgf("answers: %v", answers)

	is.Equal(len(answers), 11)
}

func TestAnagramRangeSmallOrderDoesntMatter(t *testing.T) {
	is := is.New(t)
	d, err := kwg.GetKWG(DefaultConfig, "CSW19")
	is.NoErr(err)
	answers := Anagram("(JQXZ)(AEIOU)", d, ModeExact)
	log.Info().Msgf("answers: %v", answers)

	is.Equal(len(answers), 11)
}

func TestAnagramRange(t *testing.T) {
	is := is.New(t)
	d, err := kwg.GetKWG(DefaultConfig, "CSW19")
	is.NoErr(err)
	answers := Anagram("AE(JQXZ)NR?(KY)?", d, ModeExact)
	log.Info().Msgf("answers: %v", answers)

	is.Equal(len(answers), 8)
}

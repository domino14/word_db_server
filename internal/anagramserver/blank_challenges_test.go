package anagramserver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/domino14/macondo/alphabet"
	mcconfig "github.com/domino14/macondo/config"
	"github.com/domino14/macondo/gaddag"
	"github.com/domino14/macondo/gaddagmaker"
	pb "github.com/domino14/word_db_server/rpc/wordsearcher"
	"github.com/stretchr/testify/assert"
)

var DefaultConfig = mcconfig.Config{
	StrategyParamsPath:        os.Getenv("STRATEGY_PARAMS_PATH"),
	LexiconPath:               os.Getenv("LEXICON_PATH"),
	LetterDistributionPath:    os.Getenv("LETTER_DISTRIBUTION_PATH"),
	DefaultLexicon:            "NWL18",
	DefaultLetterDistribution: "English",
}

func TestMain(m *testing.M) {
	for _, lex := range []string{"America", "FISE2"} {
		gdgPath := filepath.Join(DefaultConfig.LexiconPath, "dawg", lex+".dawg")
		if _, err := os.Stat(gdgPath); os.IsNotExist(err) {
			gaddagmaker.GenerateDawg(filepath.Join(DefaultConfig.LexiconPath, lex+".txt"), true, true, false)
			err = os.Rename("out.dawg", gdgPath)
			if err != nil {
				panic(err)
			}
		}
	}
	os.Exit(m.Run())
}

func loadDawg(lexName string) (*gaddag.SimpleDawg, error) {
	return gaddag.LoadDawg(filepath.Join(DefaultConfig.LexiconPath, "dawg", lexName+".dawg"))
}

func TestRacks(t *testing.T) {
	eng, err := loadDawg("America")
	assert.Nil(t, err)
	span, err := loadDawg("FISE2")
	assert.Nil(t, err)
	engAlph := eng.GetAlphabet()
	spanAlph := span.GetAlphabet()

	eld, err := alphabet.Get(&DefaultConfig, "english")
	if err != nil {
		t.Error(err)
	}

	sld, err := alphabet.Get(&DefaultConfig, "spanish")
	if err != nil {
		t.Error(err)
	}

	dists := []*alphabet.LetterDistribution{eld, sld}

	for distIdx, dist := range dists {
		for l := int32(7); l <= 8; l++ {
			for n := int32(1); n <= 2; n++ {
				var alph *alphabet.Alphabet
				if distIdx == 0 {
					alph = engAlph
				} else {
					alph = spanAlph
				}
				for i := 0; i < 10000; i++ {
					rack := genRack(dist, l, n, alph)
					if int32(len(rack)) != l {
						t.Errorf("Len rack should have been %v, was %v",
							l, len(rack))
					}
					numBlanks := 0
					for j := 0; j < len(rack); j++ {
						if rack[j] == alphabet.BlankMachineLetter {
							numBlanks++
						}
					}
					if int32(numBlanks) != n {
						t.Errorf("Should have had exactly %v blanks, got %v",
							n, numBlanks)
					}
				}
			}
		}
	}
}

func TestGenBlanks(t *testing.T) {
	ctx := context.Background()

	req := &pb.BlankChallengeCreateRequest{
		Lexicon:         "America",
		WordLength:      7,
		NumQuestions:    25,
		MaxSolutions:    5,
		NumWith_2Blanks: 6,
	}

	qs, err := GenerateBlanks(ctx, &DefaultConfig, req)
	if err != nil {
		t.Errorf("GenBlanks returned an error: %v", err)
	}
	num2Blanks := int32(0)
	if int32(len(qs)) != req.NumQuestions {
		t.Errorf("Generated %v questions, expected %v", len(qs), req.NumQuestions)
	}
	for _, q := range qs {
		if strings.Count(q.Alphagram, "?") == 2 {
			num2Blanks++
		}
		if int32(len(q.Words)) > req.MaxSolutions {
			t.Errorf("Number of solutions was %v, expected <= %v", len(q.Words),
				req.MaxSolutions)
		}
	}
	if num2Blanks != req.NumWith_2Blanks {
		t.Errorf("Expected %v 2-blank questions, got %v", req.NumWith_2Blanks,
			num2Blanks)
	}
}

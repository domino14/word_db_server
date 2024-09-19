package anagramserver

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/domino14/word-golib/config"
	"github.com/domino14/word-golib/kwg"
	"github.com/domino14/word-golib/tilemapping"

	pb "github.com/domino14/word_db_server/rpc/api/wordsearcher"
)

var DefaultConfig = &config.Config{
	DataPath: os.Getenv("WDB_DATA_PATH"),
}

func loadKWG(lexName string) (*kwg.KWG, error) {
	return kwg.Get(DefaultConfig, lexName)
}

func TestRacks(t *testing.T) {
	eng, err := loadKWG("America")
	assert.Nil(t, err)
	span, err := loadKWG("FISE2")
	assert.Nil(t, err)
	engAlph := eng.GetAlphabet()
	spanAlph := span.GetAlphabet()

	eld, err := tilemapping.GetDistribution(DefaultConfig, "english")
	if err != nil {
		t.Error(err)
	}

	sld, err := tilemapping.GetDistribution(DefaultConfig, "spanish")
	if err != nil {
		t.Error(err)
	}

	dists := []*tilemapping.LetterDistribution{eld, sld}

	for distIdx, dist := range dists {
		for l := int32(7); l <= 8; l++ {
			for n := int32(1); n <= 2; n++ {
				var alph *tilemapping.TileMapping
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
						if rack[j] == 0 {
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

	qs, err := GenerateBlanks(ctx, DefaultConfig, req)
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

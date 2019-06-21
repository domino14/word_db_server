package anagramserver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/domino14/macondo/alphabet"
	"github.com/domino14/macondo/anagrammer"
	"github.com/domino14/macondo/gaddag"
	"github.com/domino14/macondo/gaddagmaker"
	pb "github.com/domino14/word_db_server/rpc/anagrammer"
)

var LexiconPath = os.Getenv("LEXICON_PATH")

func TestMain(m *testing.M) {
	os.MkdirAll("/tmp/dawg", os.ModePerm)
	if _, err := os.Stat("/tmp/dawg/gen_america.dawg"); os.IsNotExist(err) {
		gaddagmaker.GenerateDawg(filepath.Join(LexiconPath, "America.txt"), true, true)
		os.Rename("out.dawg", "/tmp/dawg/gen_america.dawg")
	}
	if _, err := os.Stat("/tmp/dawg/gen_fise2.dawg"); os.IsNotExist(err) {
		gaddagmaker.GenerateDawg(filepath.Join(LexiconPath, "FISE2.txt"), true, true)
		os.Rename("out.dawg", "/tmp/dawg/gen_fise2.dawg")
	}

	os.Exit(m.Run())
}

func TestRacks(t *testing.T) {
	eng := gaddag.LoadGaddag("/tmp/dawg/gen_america.dawg")
	span := gaddag.LoadGaddag("/tmp/dawg/gen_fise2.dawg")
	engAlph := eng.GetAlphabet()
	spanAlph := span.GetAlphabet()
	dists := []*alphabet.LetterDistribution{
		alphabet.EnglishLetterDistribution(),
		alphabet.SpanishLetterDistribution(),
	}
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
	anagrammer.LoadDawgs("/tmp/dawg")

	req := &pb.BlankChallengeCreateRequest{
		Lexicon:         "America",
		WordLength:      7,
		NumQuestions:    25,
		MaxSolutions:    5,
		NumWith_2Blanks: 6,
	}
	qs, err := GenerateBlanks(ctx, req)
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

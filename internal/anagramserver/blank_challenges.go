// blank_challenges has utilities for generating racks with blanks
// that have 1 or more solutions.
package anagramserver

import (
	"context"
	"fmt"

	"github.com/rs/zerolog/log"

	"github.com/domino14/macondo/alphabet"
	"github.com/domino14/macondo/anagrammer"
	"github.com/domino14/macondo/gaddag"
	pb "github.com/domino14/word_db_server/rpc/anagrammer"
	"github.com/domino14/word_db_server/rpc/wordsearcher"
)

// try tries to generate challenges. It returns an error if it fails
// to generate a challenge with too many or too few answers, or if
// an answer has already been generated.
func try(nBlanks int32, dist *alphabet.LetterDistribution, wordLength int32,
	dawg *gaddag.SimpleGaddag, maxSolutions int32, answerMap map[string]bool) (
	*wordsearcher.Alphagram, error) {

	alph := dawg.GetAlphabet()
	rack := alphabet.MachineWord(genRack(dist, wordLength, nBlanks, alph))
	answers := anagrammer.Anagram(rack.UserVisible(alph), dawg, anagrammer.ModeExact)
	if len(answers) == 0 || int32(len(answers)) > maxSolutions {
		// Try again!
		return nil, fmt.Errorf("too many or few answers: %v %v",
			len(answers), rack.UserVisible(alph))
	}
	for _, answer := range answers {
		if answerMap[answer] {
			return nil, fmt.Errorf("duplicate answer %v", answer)
		}
	}
	for _, answer := range answers {
		answerMap[answer] = true
	}
	w := alphabet.Word{Word: rack.UserVisible(alph), Dist: dist}

	return &wordsearcher.Alphagram{
		Alphagram: w.MakeAlphagram(),
		Words:     wordsToPBWords(answers),
	}, nil

}

// GenerateBlanks - Generate a list of blank word challenges given the
// parameters in args.
func GenerateBlanks(ctx context.Context, req *pb.BlankChallengeCreateRequest) (
	[]*wordsearcher.Alphagram, error) {

	dinfo, ok := anagrammer.Dawgs[req.Lexicon]
	if !ok {
		return nil, fmt.Errorf("lexicon %v not found", req.Lexicon)
	}

	tries := 0
	// Handle 2-blank challenges at the end.
	// First gen 1-blank challenges.
	answerMap := make(map[string]bool)

	questions := []*wordsearcher.Alphagram{}
	qIndex := int32(0)

	defer func() {
		log.Debug().Msg("Leaving GenerateBlanks")
	}()
	doIteration := func() (*wordsearcher.Alphagram, error) {
		if qIndex < req.NumQuestions-req.NumWith_2Blanks {
			question, err := try(1, dinfo.GetDist(), req.WordLength, dinfo.GetDawg(),
				req.MaxSolutions, answerMap)
			tries++
			return question, err
		} else if qIndex < req.NumQuestions {
			question, err := try(2, dinfo.GetDist(), req.WordLength, dinfo.GetDawg(),
				req.MaxSolutions, answerMap)
			tries++
			return question, err
		}
		return nil, fmt.Errorf("iteration failed?")
	}

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()

		default:
			question, err := doIteration()
			if err != nil {
				log.Debug().Err(err).Msg("")
				continue
			}
			questions = append(questions, question)
			qIndex++
			if int32(len(questions)) == req.NumQuestions {
				log.Info().Msgf("%v tries", tries)
				return questions, nil
			}
		}
	}

}

// genRack - Generate a random rack using `dist` and with `blanks` blanks.
func genRack(dist *alphabet.LetterDistribution, wordLength, blanks int32,
	alph *alphabet.Alphabet) []alphabet.MachineLetter {

	bag := dist.MakeBag(alph)
	// it's a bag of machine letters.
	rack := make([]alphabet.MachineLetter, wordLength)
	idx := int32(0)
	draw := func(avoidBlanks bool) alphabet.MachineLetter {
		var tiles []alphabet.MachineLetter
		if avoidBlanks {
			for tiles, _ = bag.Draw(1); tiles[0] == alphabet.BlankMachineLetter; {
				tiles, _ = bag.Draw(1)
			}
		} else {
			tiles, _ = bag.Draw(1)
		}
		return tiles[0]
	}
	for idx < wordLength-blanks {
		// Avoid blanks on draw if user specifies a number of blanks.
		rack[idx] = draw(blanks != 0)
		idx++
	}
	for ; idx < wordLength; idx++ {
		rack[idx] = alphabet.BlankMachineLetter
	}
	return rack
}

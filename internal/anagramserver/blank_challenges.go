// blank_challenges has utilities for generating racks with blanks
// that have 1 or more solutions.
package anagramserver

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/domino14/word-golib/config"
	"github.com/domino14/word-golib/kwg"
	"github.com/domino14/word-golib/tilemapping"
	"github.com/rs/zerolog/log"

	pb "github.com/domino14/word_db_server/api/rpc/wordsearcher"
	"github.com/domino14/word_db_server/internal/common"
)

var randSource = rand.New(rand.NewSource(time.Now().UnixNano()))

// try tries to generate challenges. It returns an error if it fails
// to generate a challenge with too many or too few answers, or if
// an answer has already been generated.
func try(nBlanks int32, dist *tilemapping.LetterDistribution, wordLength int32,
	thedawg *kwg.KWG, maxSolutions int32, answerMap map[string]bool) (
	*pb.Alphagram, error) {

	alph := thedawg.GetAlphabet()
	rack := tilemapping.MachineWord(genRack(dist, wordLength, nBlanks, alph))

	da := kwg.DaPool.Get().(*kwg.KWGAnagrammer)
	defer kwg.DaPool.Put(da)

	err := da.InitForMachineWord(thedawg, rack)
	if err != nil {
		return nil, err
	}
	var answers []string
	da.Anagram(thedawg, func(word tilemapping.MachineWord) error {
		answers = append(answers, word.UserVisible(alph))
		return nil
	})

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
	w := common.InitializeWord(rack.UserVisible(alph), dist)

	return &pb.Alphagram{
		Alphagram: w.MakeAlphagram(),
		Words:     wordsToPBWords(answers),
	}, nil

}

// GenerateBlanks - Generate a list of blank word challenges given the
// parameters in args.
func GenerateBlanks(ctx context.Context, cfg *config.Config, req *pb.BlankChallengeCreateRequest) (
	[]*pb.Alphagram, error) {

	dawg, err := kwg.Get(cfg, req.Lexicon)
	if err != nil {
		return nil, err
	}
	dist, err := tilemapping.ProbableLetterDistribution(cfg, req.Lexicon)
	if err != nil {
		return nil, err
	}

	tries := 0
	// Handle 2-blank challenges at the end.
	// First gen 1-blank challenges.
	answerMap := make(map[string]bool)

	questions := []*pb.Alphagram{}
	qIndex := int32(0)

	defer func() {
		log.Debug().Msg("Leaving GenerateBlanks")
	}()
	doIteration := func() (*pb.Alphagram, error) {
		if qIndex < req.NumQuestions-req.NumWith_2Blanks {
			question, err := try(1, dist, req.WordLength, dawg,
				req.MaxSolutions, answerMap)
			tries++
			return question, err
		} else if qIndex < req.NumQuestions {
			question, err := try(2, dist, req.WordLength, dawg,
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
func genRack(dist *tilemapping.LetterDistribution, wordLength, blanks int32,
	alph *tilemapping.TileMapping) []tilemapping.MachineLetter {

	bag := dist.MakeBag()
	// it's a bag of machine letters.
	rack := make([]tilemapping.MachineLetter, wordLength)
	idx := int32(0)
	draw := func(avoidBlanks bool) tilemapping.MachineLetter {
		tiles := make([]tilemapping.MachineLetter, 1)
		if avoidBlanks {
			for _ = bag.Draw(1, tiles); tiles[0] == 0; {
				_ = bag.Draw(1, tiles)
			}
		} else {
			_ = bag.Draw(1, tiles)
		}
		return tiles[0]
	}
	for idx < wordLength-blanks {
		// Avoid blanks on draw if user specifies a number of blanks.
		rack[idx] = draw(blanks != 0)
		idx++
	}
	for ; idx < wordLength; idx++ {
		rack[idx] = 0
	}
	return rack
}

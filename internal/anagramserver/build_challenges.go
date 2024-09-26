package anagramserver

import (
	"context"
	"fmt"

	"github.com/domino14/word-golib/config"
	"github.com/domino14/word-golib/kwg"
	"github.com/domino14/word-golib/tilemapping"
	"github.com/rs/zerolog/log"

	pb "github.com/domino14/word_db_server/api/rpc/wordsearcher"
	"github.com/domino14/word_db_server/internal/common"
)

// GenerateBuildChallenge generates a build challenge with given args.
// As an additional condition, letters must anagram exactly to at least
// one word, if that argument is passed in.
func GenerateBuildChallenge(ctx context.Context, cfg *config.Config, req *pb.BuildChallengeCreateRequest) (
	*pb.Alphagram, error) {

	dawg, err := kwg.Get(cfg, req.Lexicon)
	if err != nil {
		return nil, err
	}
	dist, err := tilemapping.ProbableLetterDistribution(cfg, req.Lexicon)
	if err != nil {
		return nil, err
	}

	tries := 0
	alph := dawg.GetAlphabet()

	da := kwg.DaPool.Get().(*kwg.KWGAnagrammer)
	defer kwg.DaPool.Put(da)

	doIteration := func() (*pb.Alphagram, error) {
		rack := tilemapping.MachineWord(genRack(dist, req.MaxLength, 0, alph))
		tries++

		err := da.InitForMachineWord(dawg, rack)
		if err != nil {
			return nil, err
		}
		nanag := 0
		da.Anagram(dawg, func(word tilemapping.MachineWord) error {
			nanag += 1
			return nil
		})

		if nanag == 0 && req.RequireLengthSolution {
			return nil, fmt.Errorf("exact required and not found: %v", rack.UserVisible(alph))
		}

		var answers []string
		da.Subanagram(dawg, func(word tilemapping.MachineWord) error {
			answers = append(answers, word.UserVisible(alph))
			return nil
		})

		if int32(len(answers)) < req.MinSolutions {
			return nil, fmt.Errorf("total answers fewer than min solutions: %v < %v",
				len(answers), req.MinSolutions)
		}
		meetingCriteria := []string{}
		for _, answer := range answers {
			// NB: This might be the only place where we need to use
			// len([]rune(x)) instead of len(x). It's important to use
			// `MachineLetter`s everywhere we can.
			if int32(len([]rune(answer))) >= req.MinLength {
				meetingCriteria = append(meetingCriteria, answer)
			}
		}
		if int32(len(meetingCriteria)) < req.MinSolutions ||
			int32(len(meetingCriteria)) > req.MaxSolutions {
			return nil, fmt.Errorf("answers (%v) not match criteria: %v - %v",
				len(meetingCriteria), req.MinSolutions, req.MaxSolutions)
		}
		w := common.InitializeWord(rack.UserVisible(alph), dist)
		return &pb.Alphagram{
			Alphagram: w.MakeAlphagram(),
			Words:     wordsToPBWords(meetingCriteria),
		}, nil
	}

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Could not generate before deadline, exiting.")
			return nil, ctx.Err()
		default:
			question, err := doIteration()
			if err != nil {
				log.Debug().Err(err).Msg("")
				continue
			}
			log.Info().Msgf("%v tries", tries)
			return question, nil
		}
	}
}

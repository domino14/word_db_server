package anagramserver

import (
	"context"
	"fmt"

	"github.com/domino14/macondo/alphabet"
	"github.com/domino14/macondo/anagrammer"
	mcconfig "github.com/domino14/macondo/config"
	"github.com/domino14/word_db_server/internal/dawg"
	pb "github.com/domino14/word_db_server/rpc/wordsearcher"
	"github.com/rs/zerolog/log"
)

// GenerateBuildChallenge generates a build challenge with given args.
// As an additional condition, letters must anagram exactly to at least
// one word, if that argument is passed in.
func GenerateBuildChallenge(ctx context.Context, cfg *mcconfig.Config, req *pb.BuildChallengeCreateRequest) (
	*pb.Alphagram, error) {

	dinfo, err := dawg.GetDawgInfo(cfg, req.Lexicon)
	if err != nil {
		return nil, err
	}

	tries := 0
	alph := dinfo.GetDawg().GetAlphabet()

	doIteration := func() (*pb.Alphagram, error) {
		rack := alphabet.MachineWord(genRack(dinfo.GetDist(), req.MaxLength, 0, alph))
		tries++
		answers := anagrammer.Anagram(rack.UserVisible(alph), dinfo.GetDawg(), anagrammer.ModeExact)
		if len(answers) == 0 && req.RequireLengthSolution {
			return nil, fmt.Errorf("exact required and not found: %v", rack.UserVisible(alph))
		}
		answers = anagrammer.Anagram(rack.UserVisible(alph), dinfo.GetDawg(), anagrammer.ModeBuild)
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
		w := alphabet.Word{Word: rack.UserVisible(alph), Dist: dinfo.GetDist()}
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

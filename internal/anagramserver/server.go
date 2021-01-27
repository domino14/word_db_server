package anagramserver

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/domino14/macondo/anagrammer"
	mcconfig "github.com/domino14/macondo/config"

	"github.com/domino14/word_db_server/internal/dawg"
	"github.com/domino14/word_db_server/internal/searchserver"
	pb "github.com/domino14/word_db_server/rpc/wordsearcher"
	"github.com/rs/zerolog/log"
	"github.com/twitchtv/twirp"
)

const (
	// BlankQuestionsTimeout - how much time to give blank challenge
	// generator before giving up
	BlankQuestionsTimeout = 5000 * time.Millisecond
	// BuildQuestionsTimeout - how much time to give build challenge
	// generator before giving up
	BuildQuestionsTimeout = 5000 * time.Millisecond
)

type Server struct {
	MacondoConfig *mcconfig.Config
}

func timeTrack(start time.Time, name string) {
	elapsed := time.Since(start)
	log.Info().Msgf("%s took %s", name, elapsed)
}

func wordsToPBWords(strs []string) []*pb.Word {
	words := []*pb.Word{}
	for _, s := range strs {
		words = append(words, &pb.Word{
			Word: s,
		})
	}
	return words
}

func expandWords(ctx context.Context, ss *searchserver.Server,
	req *pb.SearchResponse) ([]*pb.Word, error) {

	expansion, err := ss.Expand(ctx, req)
	if err != nil {
		return nil, err
	}
	if len(expansion.Alphagrams) != 1 {
		return nil, errors.New("expansion failed, alphagrams length not 1")
	}
	return expansion.Alphagrams[0].Words, nil
}

func (s *Server) Anagram(ctx context.Context, req *pb.AnagramRequest) (
	*pb.AnagramResponse, error) {
	defer timeTrack(time.Now(), "anagram")

	dawgInfo, err := dawg.GetDawgInfo(s.MacondoConfig, req.Lexicon)
	if err != nil {
		return nil, err
	}

	var mode anagrammer.AnagramMode
	switch req.Mode {
	case pb.AnagramRequest_EXACT:
		mode = anagrammer.ModeExact
	case pb.AnagramRequest_BUILD:
		mode = anagrammer.ModeBuild
	}
	if strings.Count(req.Letters, "?") > 8 {
		// XXX: Add auth key?
		return nil, errors.New("query too complex")
	}
	sols := anagrammer.Anagram(req.Letters, dawgInfo.GetDawg(), mode)
	var words []*pb.Word
	if req.Expand && len(sols) > 0 {
		// Build an expand request.
		expander := &searchserver.Server{
			Config: s.MacondoConfig,
		}
		alphagram := &pb.Alphagram{
			Alphagram: req.Letters, // not technically an alphagram but doesn't matter rn
			Words:     wordsToPBWords(sols),
		}
		expandReq := &pb.SearchResponse{
			Alphagrams: []*pb.Alphagram{alphagram},
			Lexicon:    req.Lexicon,
		}

		words, err = expandWords(ctx, expander, expandReq)
		if err != nil {
			return nil, err
		}
	} else {
		words = wordsToPBWords(sols)
	}

	return &pb.AnagramResponse{
		Words:    words,
		NumWords: int32(len(sols)),
	}, nil
}

func (s *Server) BlankChallengeCreator(ctx context.Context, req *pb.BlankChallengeCreateRequest) (
	*pb.SearchResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, BlankQuestionsTimeout)
	defer cancel()

	blanks, err := GenerateBlanks(ctx, s.MacondoConfig, req)
	if err == context.DeadlineExceeded {
		// Sadly, using twirp.DeadlineExceeded results in a 408 status code,
		// which causes web browsers to keep trying request again!
		return nil, twirp.NewError(twirp.Internal, "blank challenge timed out")
	}
	if err != nil {
		return nil, err
	}
	return &pb.SearchResponse{
		Alphagrams: blanks,
		Lexicon:    req.Lexicon,
	}, nil

}

func (s *Server) BuildChallengeCreator(ctx context.Context, req *pb.BuildChallengeCreateRequest) (
	*pb.SearchResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, BuildQuestionsTimeout)
	defer cancel()
	question, err := GenerateBuildChallenge(ctx, s.MacondoConfig, req)
	if err == context.DeadlineExceeded {
		return nil, twirp.NewError(twirp.DeadlineExceeded, "build challenge timed out")
	}
	if err != nil {
		return nil, err
	}
	return &pb.SearchResponse{
		// A 1-element array is fine.
		Alphagrams: []*pb.Alphagram{question},
		Lexicon:    req.Lexicon,
	}, nil
}

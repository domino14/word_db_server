package anagramserver

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/domino14/macondo/anagrammer"
	pb "github.com/domino14/word_db_server/rpc/anagrammer"
	"github.com/domino14/word_db_server/rpc/wordsearcher"
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
	LexiconPath string
}

func timeTrack(start time.Time, name string) {
	elapsed := time.Since(start)
	log.Info().Msgf("%s took %s", name, elapsed)
}

func (s *Server) Initialize() {
	// Initialize the Macondo anagrammer.
	anagrammer.LoadDawgs(filepath.Join(s.LexiconPath, "dawg"))
}

func wordsToPBWords(strs []string) []*wordsearcher.Word {
	words := []*wordsearcher.Word{}
	for _, s := range strs {
		words = append(words, &wordsearcher.Word{
			Word: s,
		})
	}
	return words
}

func (s *Server) Anagram(ctx context.Context, req *pb.AnagramRequest) (
	*pb.AnagramResponse, error) {
	defer timeTrack(time.Now(), "anagram")

	dinfo, ok := anagrammer.Dawgs[req.Lexicon]
	if !ok {
		return nil, fmt.Errorf("lexicon %v not found", req.Lexicon)
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
	sols := anagrammer.Anagram(req.Letters, dinfo.GetDawg(), mode)

	return &pb.AnagramResponse{
		Words:    wordsToPBWords(sols),
		NumWords: int32(len(sols)),
	}, nil
}

func (s *Server) BlankChallengeCreator(ctx context.Context, req *pb.BlankChallengeCreateRequest) (
	*wordsearcher.SearchResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, BlankQuestionsTimeout)
	defer cancel()

	blanks, err := GenerateBlanks(ctx, req)
	if err == context.DeadlineExceeded {
		return nil, twirp.NewError(twirp.DeadlineExceeded, "blank challenge timed out")
	}
	if err != nil {
		return nil, err
	}
	return &wordsearcher.SearchResponse{
		Alphagrams: blanks,
		Lexicon:    req.Lexicon,
	}, nil

}

func (s *Server) BuildChallengeCreator(ctx context.Context, req *pb.BuildChallengeCreateRequest) (
	*wordsearcher.SearchResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, BuildQuestionsTimeout)
	defer cancel()
	question, err := GenerateBuildChallenge(ctx, req)
	if err == context.DeadlineExceeded {
		return nil, twirp.NewError(twirp.DeadlineExceeded, "build challenge timed out")
	}
	if err != nil {
		return nil, err
	}
	return &wordsearcher.SearchResponse{
		// A 1-element array is fine.
		Alphagrams: []*wordsearcher.Alphagram{question},
		Lexicon:    req.Lexicon,
	}, nil
}

package anagramserver

import (
	"context"
	"errors"
	"strings"
	"time"

	"connectrpc.com/connect"
	wglconfig "github.com/domino14/word-golib/config"
	"github.com/domino14/word-golib/kwg"
	"github.com/domino14/word-golib/tilemapping"

	pb "github.com/domino14/word_db_server/api/rpc/wordsearcher"
	"github.com/domino14/word_db_server/config"
	anagrammer "github.com/domino14/word_db_server/internal/anagramserver/legacyanagrammer"
	"github.com/domino14/word_db_server/internal/searchserver"
	"github.com/rs/zerolog/log"
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
	Config    *wglconfig.Config
	WDBConfig *config.Config
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

	expansion, err := ss.Expand(ctx, connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	if len(expansion.Msg.Alphagrams) != 1 {
		return nil, errors.New("expansion failed, alphagrams length not 1")
	}
	return expansion.Msg.Alphagrams[0].Words, nil
}

func (s *Server) Anagram(ctx context.Context, req *connect.Request[pb.AnagramRequest]) (
	*connect.Response[pb.AnagramResponse], error) {
	defer timeTrack(time.Now(), "anagram")

	dawg, err := kwg.GetKWG(s.Config, req.Msg.Lexicon)
	if err != nil {
		return nil, err
	}

	var sols []string
	if strings.Contains(req.Msg.Letters, "(") {
		// defer to the legacy anagrammer. This is a "range" query.
		if req.Msg.Mode == pb.AnagramRequest_SUPER {
			return nil, errors.New("cannot use super-anagram mode with range queries")
		}
		sols = anagrammer.Anagram(req.Msg.Letters, dawg, anagrammer.AnagramMode(req.Msg.Mode))
	} else {

		da := kwg.DaPool.Get().(*kwg.KWGAnagrammer)
		defer kwg.DaPool.Put(da)

		var anagFunc func(dawg *kwg.KWG, f func(tilemapping.MachineWord) error) error
		switch req.Msg.Mode {
		case pb.AnagramRequest_EXACT:
			anagFunc = da.Anagram
		case pb.AnagramRequest_BUILD:
			anagFunc = da.Subanagram
		case pb.AnagramRequest_SUPER:
			anagFunc = da.Superanagram
		}
		if strings.Count(req.Msg.Letters, "?") > 8 {
			// XXX: Add auth key?
			return nil, errors.New("query too complex; try using Super-anagram mode instead")
		}
		alph := dawg.GetAlphabet()
		err = da.InitForString(dawg, strings.ToUpper(req.Msg.Letters))
		if err != nil {
			return nil, err
		}

		anagFunc(dawg, func(word tilemapping.MachineWord) error {
			sols = append(sols, word.UserVisible(alph))
			return nil
		})
	}

	var words []*pb.Word
	if req.Msg.Expand && len(sols) > 0 {
		// Build an expand request.
		expander := &searchserver.Server{
			Config: s.WDBConfig,
		}
		alphagram := &pb.Alphagram{
			Alphagram: req.Msg.Letters, // not technically an alphagram but doesn't matter rn
			Words:     wordsToPBWords(sols),
		}
		expandReq := &pb.SearchResponse{
			Alphagrams: []*pb.Alphagram{alphagram},
			Lexicon:    req.Msg.Lexicon,
		}

		words, err = expandWords(ctx, expander, expandReq)
		if err != nil {
			return nil, err
		}
	} else {
		words = wordsToPBWords(sols)
	}

	return connect.NewResponse(&pb.AnagramResponse{
		Words:    words,
		NumWords: int32(len(sols)),
	}), nil
}

func (s *Server) BlankChallengeCreator(ctx context.Context, req *connect.Request[pb.BlankChallengeCreateRequest]) (
	*connect.Response[pb.SearchResponse], error) {
	ctx, cancel := context.WithTimeout(ctx, BlankQuestionsTimeout)
	defer cancel()

	blanks, err := GenerateBlanks(ctx, s.Config, req.Msg)
	if err == context.DeadlineExceeded {
		// DeadlineExceeded might result in a 408 status code?
		// which causes web browsers to keep trying request again!
		return nil, connect.NewError(connect.CodeInternal, errors.New("blank challenge timed out"))
	}
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&pb.SearchResponse{
		Alphagrams: blanks,
		Lexicon:    req.Msg.Lexicon,
	}), nil

}

func (s *Server) BuildChallengeCreator(ctx context.Context, req *connect.Request[pb.BuildChallengeCreateRequest]) (
	*connect.Response[pb.SearchResponse], error) {
	ctx, cancel := context.WithTimeout(ctx, BuildQuestionsTimeout)
	defer cancel()
	question, err := GenerateBuildChallenge(ctx, s.Config, req.Msg)
	if err == context.DeadlineExceeded {
		return nil, connect.NewError(connect.CodeInternal, errors.New("build challenge timed out"))
	}
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&pb.SearchResponse{
		// A 1-element array is fine.
		Alphagrams: []*pb.Alphagram{question},
		Lexicon:    req.Msg.Lexicon,
	}), nil
}
